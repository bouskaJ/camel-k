/*
Licensed to the Apache Software Foundation (ASF) under one or more
contributor license agreements.  See the NOTICE file distributed with
this work for additional information regarding copyright ownership.
The ASF licenses this file to You under the Apache License, Version 2.0
(the "License"); you may not use this file except in compliance with
the License.  You may obtain a copy of the License at

   http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package trait

import (
	"context"
	"testing"

	"github.com/rs/xid"

	"github.com/scylladb/go-set/strset"

	"github.com/apache/camel-k/pkg/apis/camel/v1alpha1"
	"github.com/apache/camel-k/pkg/util/kubernetes"
	"github.com/apache/camel-k/pkg/util/test"

	"github.com/stretchr/testify/assert"

	routev1 "github.com/openshift/api/route/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func createTestRouteEnvironment(t *testing.T, name string) *Environment {
	catalog, err := test.DefaultCatalog()
	assert.Nil(t, err)

	return &Environment{
		CamelCatalog: catalog,
		Catalog:      NewCatalog(context.TODO(), nil),
		Integration: &v1alpha1.Integration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: "test-ns",
			},
			Status: v1alpha1.IntegrationStatus{
				Phase: v1alpha1.IntegrationPhaseDeploying,
			},
			Spec: v1alpha1.IntegrationSpec{},
		},
		IntegrationKit: &v1alpha1.IntegrationKit{
			Status: v1alpha1.IntegrationKitStatus{
				Phase: v1alpha1.IntegrationKitPhaseReady,
			},
		},
		Platform: &v1alpha1.IntegrationPlatform{
			Spec: v1alpha1.IntegrationPlatformSpec{
				Cluster: v1alpha1.IntegrationPlatformClusterOpenShift,
				Build: v1alpha1.IntegrationPlatformBuildSpec{
					PublishStrategy: v1alpha1.IntegrationPlatformBuildPublishStrategyS2I,
					Registry:        v1alpha1.IntegrationPlatformRegistrySpec{Address: "registry"},
				},
			},
		},
		EnvVars:        make([]corev1.EnvVar, 0),
		ExecutedTraits: make([]Trait, 0),
		Classpath:      strset.New(),
		Resources: kubernetes.NewCollection(
			&corev1.Service{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Service",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: "test-ns",
					Labels: map[string]string{
						"camel.apache.org/integration":  name,
						"camel.apache.org/service.type": v1alpha1.ServiceTypeUser,
					},
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{},
					Selector: map[string]string{
						"camel.apache.org/integration": name,
					},
				},
			},
		),
	}
}

func TestRoute_Default(t *testing.T) {
	name := xid.New().String()
	environment := createTestRouteEnvironment(t, name)
	traitsCatalog := environment.Catalog

	err := traitsCatalog.apply(environment)

	assert.Nil(t, err)
	assert.NotEmpty(t, environment.ExecutedTraits)
	assert.NotNil(t, environment.GetTrait(ID("container")))
	assert.NotNil(t, environment.GetTrait(ID("route")))

	route := environment.Resources.GetRoute(func(r *routev1.Route) bool {
		return r.ObjectMeta.Name == name
	})

	assert.NotNil(t, route)
	assert.Nil(t, route.Spec.TLS)
	assert.NotNil(t, route.Spec.Port)
	assert.Equal(t, httpPortName, route.Spec.Port.TargetPort.StrVal)
}

func TestRoute_Disabled(t *testing.T) {
	name := xid.New().String()
	environment := createTestRouteEnvironment(t, name)
	environment.Integration.Spec.Traits = map[string]v1alpha1.TraitSpec{
		"route": {
			Configuration: map[string]string{
				"enabled": "false",
			},
		},
	}

	traitsCatalog := environment.Catalog
	err := traitsCatalog.apply(environment)

	assert.Nil(t, err)
	assert.NotEmpty(t, environment.ExecutedTraits)
	assert.Nil(t, environment.GetTrait(ID("route")))

	route := environment.Resources.GetRoute(func(r *routev1.Route) bool {
		return r.ObjectMeta.Name == name
	})

	assert.Nil(t, route)
}

func TestRoute_TLS(t *testing.T) {
	name := xid.New().String()
	environment := createTestRouteEnvironment(t, name)
	traitsCatalog := environment.Catalog

	environment.Integration.Spec.Traits = map[string]v1alpha1.TraitSpec{
		"route": {
			Configuration: map[string]string{
				"tls-termination": string(routev1.TLSTerminationEdge),
			},
		},
	}

	err := traitsCatalog.apply(environment)

	assert.Nil(t, err)
	assert.NotEmpty(t, environment.ExecutedTraits)
	assert.NotNil(t, environment.GetTrait(ID("route")))

	route := environment.Resources.GetRoute(func(r *routev1.Route) bool {
		return r.ObjectMeta.Name == name
	})

	assert.NotNil(t, route)
	assert.NotNil(t, route.Spec.TLS)
	assert.Equal(t, routev1.TLSTerminationEdge, route.Spec.TLS.Termination)
}

func TestRoute_WithCustomServicePort(t *testing.T) {
	name := xid.New().String()
	environment := createTestRouteEnvironment(t, name)
	environment.Integration.Spec.Traits = map[string]v1alpha1.TraitSpec{
		containerTraitID: {
			Configuration: map[string]string{
				"service-port-name": "my-port",
			},
		},
	}

	traitsCatalog := environment.Catalog
	err := traitsCatalog.apply(environment)

	assert.Nil(t, err)
	assert.NotEmpty(t, environment.ExecutedTraits)
	assert.NotNil(t, environment.GetTrait(ID("container")))
	assert.NotNil(t, environment.GetTrait(ID("route")))

	route := environment.Resources.GetRoute(func(r *routev1.Route) bool {
		return r.ObjectMeta.Name == name
	})

	assert.NotNil(t, route)
	assert.NotNil(t, route.Spec.Port)
	assert.Equal(
		t,
		environment.Integration.Spec.Traits[containerTraitID].Configuration["service-port-name"],
		route.Spec.Port.TargetPort.StrVal,
	)
}
