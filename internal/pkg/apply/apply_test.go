/*
Copyright 2019 The Kubernetes Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package apply_test

import (
	"bytes"
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/src-d/go-git.v4/plumbing/object"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/cli-experimental/internal/pkg/apply"
	"sigs.k8s.io/cli-experimental/internal/pkg/clik8s"
	"sigs.k8s.io/cli-experimental/internal/pkg/wirecli/wiretest"
)

func TestApplyEmpty(t *testing.T) {
	buf := new(bytes.Buffer)
	a, done, err := wiretest.InitializeApply(clik8s.ResourceConfigs(nil), &object.Commit{}, buf)
	defer done()
	assert.NoError(t, err)
	r, err := a.Do()
	assert.NoError(t, err)
	assert.Equal(t, apply.Result{}, r)
}

func TestApply(t *testing.T) {
	buf := new(bytes.Buffer)
	kp := wiretest.InitializConfigProvider()
	fs, cleanup, err := wiretest.InitializeKustomization()
	defer cleanup()
	assert.NoError(t, err)
	assert.Equal(t, len(fs), 2)

	objects, err := kp.GetConfig(fs[0])
	assert.NoError(t, err)

	a, done, err := wiretest.InitializeApply(objects, &object.Commit{}, buf)
	defer done()
	assert.NoError(t, err)
	r, err := a.Do()
	assert.NoError(t, err)
	assert.Equal(t, apply.Result{objects}, r)

	updatedObjects, err := kp.GetConfig(fs[1])
	a.Resources = updatedObjects
	assert.NoError(t, err)
	r, err = a.Do()
	assert.NoError(t, err)
	assert.Equal(t, apply.Result{updatedObjects}, r)
}

func InitializeKustomizationWithPresence() ([]string, func(), error) {
	f1, err := ioutil.TempDir("/tmp", "TestApply")
	if err != nil {
		return nil, nil, err
	}
	err = ioutil.WriteFile(filepath.Join(f1, "kustomization.yaml"), []byte(`apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
configMapGenerator:
- name: test-map

inventory:
  type: ConfigMap
  configMap:
    name: inventory
    namespace: default

resources:
- not-apply-service.yaml

namespace: default
`), 0644)
	if err != nil {
		return nil, nil, err
	}

	err = ioutil.WriteFile(filepath.Join(f1, "not-apply-service.yaml"), []byte(`
apiVersion: v1
kind: Service
metadata:
  name: my-service
  annotations:
    kubectl.kubernetes.io/presence: EnsureDoesNotExist
spec:
  selector:
    app: MyApp
  ports:
  - protocol: TCP
    port: 80
    targetPort: 9376
`), 0644)
	if err != nil {
		return nil, nil, err
	}

	f2, err := ioutil.TempDir("/tmp", "TestApply")
	if err != nil {
		return nil, nil, err
	}
	err = ioutil.WriteFile(filepath.Join(f2, "kustomization.yaml"), []byte(`apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
configMapGenerator:
- name: test-map
  literals:
  - foo=bar

inventory:
  type: ConfigMap
  configMap:
    name: inventory
    namespace: default

resources:
- not-apply-service.yaml

namespace: default
`), 0644)
	if err != nil {
		return nil, nil, err
	}

	err = ioutil.WriteFile(filepath.Join(f2, "not-apply-service.yaml"), []byte(`
apiVersion: v1
kind: Service
metadata:
  name: my-service
  annotations:
    kubectl.kubernetes.io/presence: EnsureDoesNotExist
spec:
  selector:
    app: MyApp
  ports:
  - protocol: TCP
    port: 80
    targetPort: 9376
`), 0644)
	if err != nil {
		return nil, nil, err
	}

	return []string{f1, f2}, func() {
		os.RemoveAll(f1)
		os.RemoveAll(f2)
	}, nil
}

func TestApplyWithPresenceAnnotation(t *testing.T) {
	buf := new(bytes.Buffer)
	kp := wiretest.InitializConfigProvider()
	fs, cleanup, err := InitializeKustomizationWithPresence()
	defer cleanup()
	assert.NoError(t, err)
	assert.Equal(t, len(fs), 2)

	objects, err := kp.GetConfig(fs[0])
	assert.NoError(t, err)

	a, done, err := wiretest.InitializeApply(objects, &object.Commit{}, buf)
	defer done()

	serviceList := &unstructured.UnstructuredList{}
	serviceList.SetGroupVersionKind(schema.GroupVersionKind{
		Kind:    "ServiceList",
		Version: "v1",
	})
	err = a.DynamicClient.List(context.Background(), serviceList, "default", nil)
	defaultCount := len(serviceList.Items)

	assert.NoError(t, err)
	r, err := a.Do()
	assert.NoError(t, err)
	assert.Equal(t, apply.Result{objects}, r)
	err = a.DynamicClient.List(context.Background(), serviceList, "default", nil)
	assert.Equal(t, len(serviceList.Items), defaultCount)

	updatedObjects, err := kp.GetConfig(fs[1])
	a.Resources = updatedObjects
	assert.NoError(t, err)
	r, err = a.Do()
	assert.NoError(t, err)
	assert.Equal(t, apply.Result{updatedObjects}, r)
	err = a.DynamicClient.List(context.Background(), serviceList, "default", nil)
	assert.Equal(t, len(serviceList.Items), defaultCount)
}
