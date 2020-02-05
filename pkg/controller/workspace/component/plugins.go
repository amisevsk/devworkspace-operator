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
	"errors"
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

func getArtifactBrokerObjects(names model.WorkspaceProperties, podSpec *corev1.PodSpec, components []workspaceApi.ComponentSpec) ([]runtime.Object, error) {
	var k8sObjects []runtime.Object

	return k8sObjects, nil
}

// TODO : change this because we don't expect plugin metas anymore, but plugin FQNs in the config maps
func setupPluginInitContainers(names model.WorkspaceProperties, podSpec *corev1.PodSpec, components []workspaceApi.ComponentSpec) ([]runtime.Object, error) {
	var k8sObjects []runtime.Object

	type initContainerDef struct {
		imageName  string
		pluginFQNs []brokerModel.PluginFQN
	}

	// TODO
	var fqns []brokerModel.PluginFQN
	for _, component := range components {
		fqns = append(fqns, getPluginFQN(component))
	}

	for _, def := range []initContainerDef{
		{
			imageName:  "che.workspace.plugin_broker.init.image",
			pluginFQNs: []brokerModel.PluginFQN{},
		},
		{
			imageName:  "che.workspace.plugin_broker.unified.image",
			pluginFQNs: fqns,
		},
	} {
		brokerImage := config.ControllerCfg.GetProperty(def.imageName)
		if brokerImage == nil {
			return nil, errors.New("Unknown broker docker image for : " + def.imageName)
		}

		volumeMounts := []corev1.VolumeMount{
			corev1.VolumeMount{
				MountPath: "/plugins/",
				Name:      config.ControllerCfg.GetWorkspacePVCName(),
				SubPath:   names.WorkspaceId + "/plugins/",
			},
		}

		containerName := strings.ReplaceAll(
			strings.TrimSuffix(
				strings.TrimPrefix(def.imageName, "che.workspace.plugin_"),
				".image"),
			".", "-")
		args := []string{
			"-disable-push",
			"-runtime-id",
			fmt.Sprintf("%s:%s:%s", names.WorkspaceId, "default", "anonymous"),
			"--registry-address",
			config.ControllerCfg.GetPluginRegistry(),
		}

		if len(def.pluginFQNs) > 0 {

			// TODO: See how the unified broker is defined in the yaml
			// and define it the same way here.
			// See also how it is that we do not put volume =>
			// => Log on the operator side to see what there is in PluginFQNs

			configMapName := containerName + "-broker-config-map"
			configMapVolume := containerName + "-broker-config-volume"
			configMapContent, err := json.MarshalIndent(fqns, "", "")
			if err != nil {
				return nil, err
			}

			configMap := corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      configMapName,
					Namespace: names.Namespace,
					Labels: map[string]string{
						model.WorkspaceIDLabel: names.WorkspaceId,
					},
				},
				Data: map[string]string{
					"config.json": string(configMapContent),
				},
			}
			k8sObjects = append(k8sObjects, &configMap)

			volumeMounts = append(volumeMounts, corev1.VolumeMount{
				MountPath: "/broker-config/",
				Name:      configMapVolume,
				ReadOnly:  true,
			})

			podSpec.Volumes = append(podSpec.Volumes, corev1.Volume{
				Name: configMapVolume,
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: configMapName,
						},
					},
				},
			})

			args = append(args,
				"-metas",
				"/broker-config/config.json",
			)
		}

		podSpec.InitContainers = append(podSpec.InitContainers, corev1.Container{
			Name:  containerName,
			Image: *brokerImage,
			Args:  args,

			ImagePullPolicy:          corev1.PullPolicy(config.ControllerCfg.GetSidecarPullPolicy()),
			VolumeMounts:             volumeMounts,
			TerminationMessagePolicy: corev1.TerminationMessageFallbackToLogsOnError,
		})
	}
	return k8sObjects, nil
}

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
