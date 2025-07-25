/*
 * Copyright (c) 2022-2023, NVIDIA CORPORATION.  All rights reserved.
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
	cdiapi "tags.cncf.io/container-device-interface/pkg/cdi"
)

type cdiMockHandler struct{}

func NewCDIMockHandler(opts ...cdiOption) (CDIHandler, error) {
	return &cdiMockHandler{}, nil
}

func (cdi *cdiMockHandler) CreateStandardDeviceSpecFile(allocatable AllocatableDevices) error {
	return nil
}

func (cdi *cdiMockHandler) CreateClaimSpecFile(claimUID string, preparedDevices PreparedDevices) error {
	return nil
}

func (cdi *cdiMockHandler) DeleteClaimSpecFile(claimUID string) error {
	return nil
}

func (cdi *cdiMockHandler) GetStandardDevice(device *AllocatableDevice) string {
	// never gets any device
	return ""
}

func (cdi *cdiMockHandler) GetClaimDevice(claimUID string, device *AllocatableDevice, containerEdits *cdiapi.ContainerEdits) string {
	return ""
	// return cdiparser.QualifiedName(cdi.vendor, cdi.claimClass, fmt.Sprintf("%s-%s", claimUID, device.CanonicalName()))
}
