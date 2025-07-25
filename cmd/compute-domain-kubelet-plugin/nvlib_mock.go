/*
 * Copyright (c) 2021, NVIDIA CORPORATION.  All rights reserved.
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
	nvdev "github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

func newMockDeviceLib(driverRoot root) (*deviceLib, error) {
	// We construct an NVML library specifying the path to libnvidia-ml.so.1
	// explicitly so that we don't have to rely on the library path.
	nvmllib := &mockNvmlLib{}
	d := deviceLib{
		Interface:         nvdev.New(nvmllib),
		nvmllib:           nvmllib,
		driverLibraryPath: "",
		devRoot:           driverRoot.getDevRoot(),
		nvidiaSMIPath:     "",
	}
	return &d, nil
}

type mockNvmlLib struct {
	nvml.Interface
}

func (nv *mockNvmlLib) Init() nvml.Return     { return nvml.SUCCESS }
func (nv *mockNvmlLib) Shutdown() nvml.Return { return nvml.SUCCESS }
func (nv *mockNvmlLib) DeviceGetCount() (int, nvml.Return) {
	return 4, nvml.SUCCESS
}

func (nv *mockNvmlLib) SystemGetCudaDriverVersion() (int, nvml.Return) {
	return 129000, nvml.SUCCESS
}
func (nv *mockNvmlLib) SystemGetDriverVersion() (string, nvml.Return) {
	return "575.1.1", nvml.SUCCESS
}

type fakeDevice struct {
	nvml.Device
}

func (*fakeDevice) GetName() (string, nvml.Return) {
	return "fake-device", nvml.SUCCESS
}

func (*fakeDevice) IsFabricAttached() (bool, error) {
	return true, nil
}

func (*fakeDevice) GetGpuFabricInfo() (nvml.GpuFabricInfo, nvml.Return) {
	return nvml.GpuFabricInfo{
		ClusterUuid: [16]uint8{},
		CliqueId:    1,
		State:       nvml.GPU_FABRIC_STATE_COMPLETED,
	}, nvml.SUCCESS
}

func (nv *mockNvmlLib) DeviceGetHandleByIndex(int) (nvml.Device, nvml.Return) {
	return &fakeDevice{}, nvml.SUCCESS
}
func (nv *mockNvmlLib) DeviceGetHandleByUUID(string) (nvml.Device, nvml.Return) {
	return &fakeDevice{}, nvml.SUCCESS
}

type fakeExtension struct{}

func (*fakeExtension) LookupSymbol(string) error { return nil }

func (nv *mockNvmlLib) Extensions() nvml.ExtendedInterface {
	return &fakeExtension{}
}
