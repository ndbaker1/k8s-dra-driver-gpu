/*
 * Copyright (c) 2025 NVIDIA CORPORATION.  All rights reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	nvapi "github.com/NVIDIA/k8s-dra-driver-gpu/api/nvidia.com/resource/v1beta1"
	nvinformers "github.com/NVIDIA/k8s-dra-driver-gpu/pkg/nvidia.com/informers/externalversions"
)

type GetComputeDomainFunc func(uid string) (*nvapi.ComputeDomain, error)

const (
	// informerResyncPeriod defines how often the informer will resync its cache
	// with the API server. This helps ensure eventual consistency.
	informerResyncPeriod = 10 * time.Minute

	// mutationCacheTTL defines how long mutation cache entries remain valid.
	// This should be long enough for the informer cache to catch up but
	// not so long that stale entries cause issues.
	mutationCacheTTL = time.Hour

	computeDomainLabelKey  = "resource.nvidia.com/computeDomain"
	computeDomainFinalizer = computeDomainLabelKey

	computeDomainDefaultChannelDeviceClass = "compute-domain-default-channel.nvidia.com"
	computeDomainChannelDeviceClass        = "compute-domain-channel.nvidia.com"
	computeDomainDaemonDeviceClass         = "compute-domain-daemon.nvidia.com"

	computeDomainResourceClaimTemplateTargetLabelKey = "resource.nvidia.com/computeDomainTarget"
	computeDomainResourceClaimTemplateTargetDaemon   = "Daemon"
	computeDomainResourceClaimTemplateTargetWorkload = "Workload"
)

type ComputeDomainManager struct {
	config        *ManagerConfig
	waitGroup     sync.WaitGroup
	cancelContext context.CancelFunc

	factory  nvinformers.SharedInformerFactory
	informer cache.SharedIndexInformer

	daemonSetManager             *DaemonSetManager
	resourceClaimTemplateManager *WorkloadResourceClaimTemplateManager
	nodeManager                  *NodeManager
}

// NewComputeDomainManager creates a new ComputeDomainManager.
func NewComputeDomainManager(config *ManagerConfig) *ComputeDomainManager {
	factory := nvinformers.NewSharedInformerFactory(config.clientsets.Nvidia, informerResyncPeriod)
	informer := factory.Resource().V1beta1().ComputeDomains().Informer()

	m := &ComputeDomainManager{
		config:   config,
		factory:  factory,
		informer: informer,
	}
	m.daemonSetManager = NewDaemonSetManager(config, m.Get)
	m.resourceClaimTemplateManager = NewWorkloadResourceClaimTemplateManager(config, m.Get)
	m.nodeManager = NewNodeManager(config, m.Get)

	return m
}

// Start starts a ComputeDomainManager.
func (m *ComputeDomainManager) Start(ctx context.Context) (rerr error) {
	ctx, cancel := context.WithCancel(ctx)
	m.cancelContext = cancel

	defer func() {
		if rerr != nil {
			if err := m.Stop(); err != nil {
				klog.Errorf("error stopping ComputeDomain manager: %v", err)
			}
		}
	}()

	err := m.informer.AddIndexers(cache.Indexers{
		"uid": uidIndexer[*nvapi.ComputeDomain],
	})
	if err != nil {
		return fmt.Errorf("error adding indexer for UIDs: %w", err)
	}

	_, err = m.informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj any) {
			m.config.workQueue.Enqueue(obj, m.onAddOrUpdate)
		},
		UpdateFunc: func(oldObj, newObj any) {
			m.config.workQueue.Enqueue(newObj, m.onAddOrUpdate)
		},
	})
	if err != nil {
		return fmt.Errorf("error adding event handlers for ComputeDomain informer: %w", err)
	}

	m.waitGroup.Add(1)
	go func() {
		defer m.waitGroup.Done()
		m.factory.Start(ctx.Done())
	}()

	if !cache.WaitForCacheSync(ctx.Done(), m.informer.HasSynced) {
		return fmt.Errorf("informer cache sync for ComputeDomains failed")
	}

	if err := m.daemonSetManager.Start(ctx); err != nil {
		return fmt.Errorf("error starting DaemonSet manager: %w", err)
	}

	if err := m.resourceClaimTemplateManager.Start(ctx); err != nil {
		return fmt.Errorf("error creating ResourceClaim manager: %w", err)
	}

	if err := m.nodeManager.Start(ctx); err != nil {
		return fmt.Errorf("error starting Node manager: %w", err)
	}

	return nil
}

func (m *ComputeDomainManager) Stop() error {
	if err := m.daemonSetManager.Stop(); err != nil {
		return fmt.Errorf("error stopping DaemonSet manager: %w", err)
	}
	if err := m.resourceClaimTemplateManager.Stop(); err != nil {
		return fmt.Errorf("error stopping ResourceClaimTemplate manager: %w", err)
	}
	if err := m.nodeManager.Stop(); err != nil {
		return fmt.Errorf("error stopping Node manager: %w", err)
	}
	m.cancelContext()
	m.waitGroup.Wait()
	return nil
}

// Get gets a ComputeDomain with a specific UID.
func (m *ComputeDomainManager) Get(uid string) (*nvapi.ComputeDomain, error) {
	cds, err := m.informer.GetIndexer().ByIndex("uid", uid)
	if err != nil {
		return nil, fmt.Errorf("error retrieving ComputeDomain by UID: %w", err)
	}
	if len(cds) == 0 {
		return nil, nil
	}
	if len(cds) != 1 {
		return nil, fmt.Errorf("multiple ComputeDomains with the same UID")
	}
	cd, ok := cds[0].(*nvapi.ComputeDomain)
	if !ok {
		return nil, fmt.Errorf("failed to cast to ComputeDomain")
	}
	return cd, nil
}

// RemoveFinalizer removes the finalizer from a ComputeDomain.
func (m *ComputeDomainManager) RemoveFinalizer(ctx context.Context, uid string) error {
	cd, err := m.Get(uid)
	if err != nil {
		return fmt.Errorf("error retrieving ComputeDomain: %w", err)
	}
	if cd == nil {
		return nil
	}

	if cd.GetDeletionTimestamp() == nil {
		return fmt.Errorf("attempting to remove finalizer before ComputeDomain marked for deletion")
	}

	newCD := cd.DeepCopy()
	newCD.Finalizers = []string{}
	for _, f := range cd.Finalizers {
		if f != computeDomainFinalizer {
			newCD.Finalizers = append(newCD.Finalizers, f)
		}
	}
	if len(cd.Finalizers) == len(newCD.Finalizers) {
		return nil
	}

	if _, err = m.config.clientsets.Nvidia.ResourceV1beta1().ComputeDomains(cd.Namespace).Update(ctx, newCD, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("error updating ComputeDomain: %w", err)
	}

	return nil
}

func (m *ComputeDomainManager) addFinalizer(ctx context.Context, cd *nvapi.ComputeDomain) error {
	for _, f := range cd.Finalizers {
		if f == computeDomainFinalizer {
			return nil
		}
	}

	newCD := cd.DeepCopy()
	newCD.Finalizers = append(newCD.Finalizers, computeDomainFinalizer)
	if _, err := m.config.clientsets.Nvidia.ResourceV1beta1().ComputeDomains(cd.Namespace).Update(ctx, newCD, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("error updating ComputeDomain: %w", err)
	}

	return nil
}

func (m *ComputeDomainManager) onAddOrUpdate(ctx context.Context, obj any) error {
	cd, ok := obj.(*nvapi.ComputeDomain)
	if !ok {
		return fmt.Errorf("failed to cast to ComputeDomain")
	}

	klog.Infof("Processing added or updated ComputeDomain: %s/%s/%s", cd.Namespace, cd.Name, cd.UID)

	if cd.GetDeletionTimestamp() != nil {
		if err := m.resourceClaimTemplateManager.Delete(ctx, string(cd.UID)); err != nil {
			return fmt.Errorf("error deleting ResourceClaimTemplate: %w", err)
		}

		if err := m.daemonSetManager.Delete(ctx, string(cd.UID)); err != nil {
			return fmt.Errorf("error deleting DaemonSet: %w", err)
		}

		if err := m.nodeManager.RemoveComputeDomainLabels(ctx, string(cd.UID)); err != nil {
			return fmt.Errorf("error removing ComputeDomain node labels: %w", err)
		}

		if err := m.resourceClaimTemplateManager.RemoveFinalizer(ctx, string(cd.UID)); err != nil {
			return fmt.Errorf("error removing finalizer on ResourceClaimTemplate: %w", err)
		}

		if err := m.resourceClaimTemplateManager.AssertRemoved(ctx, string(cd.UID)); err != nil {
			return fmt.Errorf("error asserting removal of ResourceClaimTemplate: %w", err)
		}

		if err := m.daemonSetManager.RemoveFinalizer(ctx, string(cd.UID)); err != nil {
			return fmt.Errorf("error removing finalizer on DaemonSet: %w", err)
		}

		if err := m.daemonSetManager.AssertRemoved(ctx, string(cd.UID)); err != nil {
			return fmt.Errorf("error asserting removal of DaemonSet: %w", err)
		}

		if err := m.RemoveFinalizer(ctx, string(cd.UID)); err != nil {
			return fmt.Errorf("error removing finalizer: %w", err)
		}

		return nil
	}

	if err := m.addFinalizer(ctx, cd); err != nil {
		return fmt.Errorf("error adding finalizer: %w", err)
	}

	// Do not wait for the next periodic label cleanup to happen.
	m.nodeManager.RemoveStaleComputeDomainLabelsAsync(ctx)

	if _, err := m.daemonSetManager.Create(ctx, m.config.driverNamespace, cd); err != nil {
		return fmt.Errorf("error creating DaemonSet: %w", err)
	}

	if _, err := m.resourceClaimTemplateManager.Create(ctx, cd.Namespace, cd.Spec.Channel.ResourceClaimTemplate.Name, cd); err != nil {
		return fmt.Errorf("error creating ResourceClaimTemplate '%s/%s': %w", cd.Namespace, cd.Spec.Channel.ResourceClaimTemplate.Name, err)
	}

	return nil
}
