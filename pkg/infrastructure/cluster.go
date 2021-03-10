//
// Copyright (c) 2019-2021 Red Hat, Inc.
// This program and the accompanying materials are made
// available under the terms of the Eclipse Public License 2.0
// which is available at https://www.eclipse.org/legal/epl-2.0/
//
// SPDX-License-Identifier: EPL-2.0
//
// Contributors:
//   Red Hat, Inc. - initial API and implementation
//

package infrastructure

import (
	"fmt"
	"os"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

// Type specifies what kind of infrastructure we're operating in.
type Type int

const (
	// unsupported represents an unsupported cluster version (e.g. OpenShift v3)
	unsupported Type = iota
	kubernetes
	openshiftV4
)

var (
	// current is the infrastructure that we're currently running on.
	current Type
)

func IsOpenShift() bool {
	return current == openshiftV4
}

func init() {
	var err error
	current, err = detect()
	if err != nil {
		log.Error(err, "Failed to read cluster state: %s")
		os.Exit(1)
	}
	if current == unsupported {
		log.Error(fmt.Errorf("running on unsupported cluster"), "DevWorkspace Operator only supports Kubernetes and OpenShift v4 or greater")
		os.Exit(1)
	}
}

func detect() (Type, error) {
	kubeCfg, err := config.GetConfig()
	if err != nil {
		return unsupported, fmt.Errorf("could not get kube config: %w", err)
	}
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(kubeCfg)
	if err != nil {
		return unsupported, fmt.Errorf("could not get discovery client: %w", err)
	}
	apiList, err := discoveryClient.ServerGroups()
	if err != nil {
		return unsupported, fmt.Errorf("could not read API groups: %w", err)
	}
	if findAPIGroup(apiList.Groups, "route.openshift.io") == nil {
		return kubernetes, nil
	} else {
		if findAPIGroup(apiList.Groups, "config.openshift.io") == nil {
			return unsupported, nil
		} else {
			return openshiftV4, nil
		}
	}
}

func findAPIGroup(source []metav1.APIGroup, apiName string) *metav1.APIGroup {
	for i := 0; i < len(source); i++ {
		if source[i].Name == apiName {
			return &source[i]
		}
	}
	return nil
}

func findAPIResources(source []*metav1.APIResourceList, groupName string) []metav1.APIResource {
	for i := 0; i < len(source); i++ {
		if source[i].GroupVersion == groupName {
			return source[i].APIResources
		}
	}
	return nil
}
