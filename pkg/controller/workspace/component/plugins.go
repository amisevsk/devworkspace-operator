//
// Copyright (c) 2019-2020 Red Hat, Inc.
// This program and the accompanying materials are made
// available under the terms of the Eclipse Public License 2.0
// which is available at https://www.eclipse.org/legal/epl-2.0/
//
// SPDX-License-Identifier: EPL-2.0
//
// Contributors:
//   Red Hat, Inc. - initial API and implementation
//

package component

import (
	"encoding/json"
	"regexp"
	"strings"

	workspaceApi "github.com/che-incubator/che-workspace-crd-operator/pkg/apis/workspace/v1alpha1"
	metadataBroker "github.com/eclipse/che-plugin-broker/brokers/metadata"
	brokerModel "github.com/eclipse/che-plugin-broker/model"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"fmt"

	"github.com/che-incubator/che-workspace-crd-operator/pkg/controller/workspace/config"
	"github.com/che-incubator/che-workspace-crd-operator/pkg/controller/workspace/model"
)

var imageRegexp = regexp.MustCompile(`[^0-9a-zA-Z]`)

func setupChePlugins(props model.WorkspaceProperties, components []workspaceApi.ComponentSpec) ([]model.ComponentInstanceStatus, error) {
	var componentInstanceStatuses []model.ComponentInstanceStatus

	broker := metadataBroker.NewBroker(true)
	metas, err := getMetasForComponents(components)
	if err != nil {
		return nil, err
	}

	plugins, err := broker.ProcessPlugins(metas)
	if err != nil {
		return nil, err
	}

	for _, plugin := range plugins {
		component, err := convertToComponentInstanceStatus(plugin, props)
		if err != nil {
			return nil, err
		}
		componentInstanceStatuses = append(componentInstanceStatuses, *component)
	}

	return componentInstanceStatuses, nil
}

func getArtifactsBrokerObjects(props model.WorkspaceProperties, components []workspaceApi.ComponentSpec) (model.ComponentInstanceStatus, error) {
	var brokerComponent model.ComponentInstanceStatus

	const (
		configMapVolumeName = "broker-config-volume"
		configMapMountPath  = "/broker-config"
		configMapDataName   = "config.json"
	)
	configMapName := fmt.Sprintf("%s.broker-config-map", props.WorkspaceId)
	brokerImage := config.ControllerCfg.GetPluginArtifactsBrokerImage()
	brokerContainerName := getContainerNameFromImage(brokerImage)

	// Define plugin broker configmap
	var fqns []brokerModel.PluginFQN
	for _, component := range components {
		fqns = append(fqns, getPluginFQN(component))
	}
	cmData, err := json.Marshal(fqns)
	if err != nil {
		return brokerComponent, err
	}
	cm := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: props.Namespace,
			Labels: map[string]string{
				model.WorkspaceIDLabel: props.WorkspaceId,
			},
		},
		Data: map[string]string{
			configMapDataName: string(cmData),
		},
	}

	// Define volumes used by plugin broker
	cmVolume := corev1.Volume{
		Name: configMapVolumeName,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: configMapName,
				},
			},
		},
	}

	cmVolumeMounts := []corev1.VolumeMount{
		{
			MountPath: configMapMountPath,
			Name:      configMapVolumeName,
			ReadOnly:  true,
		},
		{
			MountPath: "/plugins",
			Name:      config.ControllerCfg.GetWorkspacePVCName(),
			SubPath:   props.WorkspaceId + "/plugins",
		},
	}

	initContainer := corev1.Container{
		Name:                     brokerContainerName,
		Image:                    brokerImage,
		ImagePullPolicy:          corev1.PullPolicy(config.ControllerCfg.GetSidecarPullPolicy()),
		VolumeMounts:             cmVolumeMounts,
		TerminationMessagePolicy: corev1.TerminationMessageFallbackToLogsOnError,
		Args: []string{
			"--disable-push",
			"--runtime-id",
			fmt.Sprintf("%s:%s:%s", props.WorkspaceId, "default", "anonymous"),
			"--registry-address",
			config.ControllerCfg.GetPluginRegistry(),
			"--metas",
			fmt.Sprintf("%s/%s", configMapMountPath, configMapDataName),
		},
	}

	brokerComponent = model.ComponentInstanceStatus{
		WorkspacePodAdditions: &corev1.PodTemplateSpec{
			Spec: corev1.PodSpec{
				InitContainers: []corev1.Container{initContainer},
				Volumes:        []corev1.Volume{cmVolume},
			},
		},
		ExternalObjects: []runtime.Object{&cm},
	}

	return brokerComponent, err
}

func getContainerNameFromImage(image string) string {
	parts := strings.Split(image, "/")
	imageName := parts[len(parts)-1]
	return imageRegexp.ReplaceAllString(imageName, "-")
}
