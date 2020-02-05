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
	broker := metadataBroker.NewBroker(true)
	metas, err := getMetasForComponents(components)
	if err != nil {
		return nil, err
	}

	_, err = broker.ProcessPlugins(metas)
	if err != nil {
		return nil, err
	}

	return nil, nil
	// var todo []*ComponentInstanceStatus
	// for _, plugin := range plugins {
	// 	componentInstance := convertToComponentInstanceStatus(plugin)
	// 	todo = append(todo, componentInstance)
	// }

	// podTemplate := &corev1.PodTemplateSpec{}
	// componentInstanceStatus.WorkspacePodAdditions = podTemplate
	// componentInstanceStatus.ExternalObjects = []runtime.Object{}

	// for _, containerDef := range chePlugin.Containers {
	// 	machineName := containerDef.Name

	// 	var exposedPorts []int = exposedPortsToInts(containerDef.Ports)

	// 	var limitOrDefault string

	// 	if containerDef.MemoryLimit == "" {
	// 		limitOrDefault = "128M"
	// 	} else {
	// 		limitOrDefault = containerDef.MemoryLimit
	// 	}

	// 	limit, err := resource.ParseQuantity(limitOrDefault)
	// 	if err != nil {
	// 		return nil, err
	// 	}

	// 	volumeMounts := createVolumeMounts(props, &containerDef.MountSources, []workspaceApi.Volume{}, containerDef.Volumes)

	// 	var envVars []corev1.EnvVar
	// 	for _, envVarDef := range containerDef.Env {
	// 		envVars = append(envVars, corev1.EnvVar{
	// 			Name:  envVarDef.Name,
	// 			Value: envVarDef.Value,
	// 		})
	// 	}
	// 	envVars = append(envVars, corev1.EnvVar{
	// 		Name:  "CHE_MACHINE_NAME",
	// 		Value: machineName,
	// 	})
	// 	container := corev1.Container{
	// 		Name:            machineName,
	// 		Image:           containerDef.Image,
	// 		ImagePullPolicy: corev1.PullPolicy(ControllerCfg.GetSidecarPullPolicy()),
	// 		Ports:           k8sModelUtils.BuildContainerPorts(exposedPorts, corev1.ProtocolTCP),
	// 		Resources: corev1.ResourceRequirements{
	// 			Limits: corev1.ResourceList{
	// 				"memory": limit,
	// 			},
	// 			Requests: corev1.ResourceList{
	// 				"memory": limit,
	// 			},
	// 		},
	// 		VolumeMounts:             volumeMounts,
	// 		Env:                      append(envVars, commonEnvironmentVariables(props)...),
	// 		TerminationMessagePolicy: corev1.TerminationMessageFallbackToLogsOnError,
	// 	}
	// 	podTemplate.Spec.Containers = append(podTemplate.Spec.Containers, container)

	// ########## Checkpoint ###########

	// for _, service := range createK8sServicesForMachines(names, machineName, exposedPorts) {
	// 	componentInstanceStatus.ExternalObjects = append(componentInstanceStatus.ExternalObjects, &service)
	// }

	// for _, endpointDef := range chePlugin.Endpoints {
	// 	attributes := map[workspaceApi.EndpointAttribute]string{}
	// 	if endpointDef.Public {
	// 		attributes[workspaceApi.PUBLIC_ENDPOINT_ATTRIBUTE] = "true"
	// 	} else {
	// 		attributes[workspaceApi.PUBLIC_ENDPOINT_ATTRIBUTE] = "false"
	// 	}
	// 	for name, value := range endpointDef.Attributes {
	// 		attributes[workspaceApi.EndpointAttribute(name)] = value
	// 	}
	// 	if attributes[workspaceApi.PROTOCOL_ENDPOINT_ATTRIBUTE] == "" {
	// 		attributes[workspaceApi.PROTOCOL_ENDPOINT_ATTRIBUTE] = "http"
	// 	}
	// 	endpoint := workspaceApi.Endpoint{
	// 		Name:       endpointDef.Name,
	// 		Port:       int64(endpointDef.TargetPort),
	// 		Attributes: attributes,
	// 	}
	// 	componentInstanceStatus.Endpoints = append(componentInstanceStatus.Endpoints, endpoint)
	// }

	// machineAttributes := map[string]string{}
	// if limitAsInt64, canBeConverted := limit.AsInt64(); canBeConverted {
	// 	machineAttributes[server.MEMORY_LIMIT_ATTRIBUTE] = strconv.FormatInt(limitAsInt64, 10)
	// 	machineAttributes[server.MEMORY_REQUEST_ATTRIBUTE] = strconv.FormatInt(limitAsInt64, 10)
	// }
	// machineAttributes[server.CONTAINER_SOURCE_ATTRIBUTE] = server.TOOL_CONTAINER_SOURCE
	// machineAttributes[server.PLUGIN_MACHINE_ATTRIBUTE] = chePlugin.ID

	// componentInstanceStatus.Machines[machineName] = MachineDescription{
	// 	MachineAttributes: machineAttributes,
	// 	Ports:             exposedPorts,
	// }

	// for _, command := range containerDef.Commands {
	// 	componentInstanceStatus.ContributedRuntimeCommands = append(componentInstanceStatus.ContributedRuntimeCommands,
	// 		CheWorkspaceCommand{
	// 			Name:        command.Name,
	// 			CommandLine: strings.Join(command.Command, " "),
	// 			Type:        "custom",
	// 			Attributes: map[string]string{
	// 				server.COMMAND_WORKING_DIRECTORY_ATTRIBUTE: interpolate(command.WorkingDir, names),
	// 				server.COMMAND_MACHINE_NAME_ATTRIBUTE:      machineName,
	// 			},
	// 		})
	// }
	// }
}

// func convertToComponentInstanceStatus(props WorkspaceProperties, plugin brokerModel.ChePlugin) (*ComponentInstanceStatus, error) {
// 	componentInstanceStatus := &ComponentInstanceStatus{
// 		Machines:                   map[string]MachineDescription{},
// 		Endpoints:                  []workspaceApi.Endpoint{},
// 		ContributedRuntimeCommands: []CheWorkspaceCommand{},
// 	}
// 	podTemplate := &corev1.PodTemplateSpec{}
// 	componentInstanceStatus.WorkspacePodAdditions = podTemplate
// 	componentInstanceStatus.ExternalObjects = []runtime.Object{}
// 	for _, pluginContainer := range plugin.Containers {
// 		container, err := toKubernetesContainer(pluginContainer, props)
// 		if err != nil {
// 			return nil, err
// 		}
// 		podTemplate.Spec.Containers = append(podTemplate.Spec.Containers, *container)
// 	}
// 	return componentInstanceStatus, nil
// }

// func toKubernetesContainer(plugin brokerModel.Container, props WorkspaceProperties) (*corev1.Container, error) {

// 	k8sContainer := &corev1.Container{
// 		Name:            plugin.Name,
// 		Image:           plugin.Image,
// 		ImagePullPolicy: corev1.PullPolicy(ControllerCfg.GetSidecarPullPolicy()),
// 		Ports:           k8sModelUtils.BuildContainerPorts(exposedPortsToInts(plugin.Ports), corev1.ProtocolTCP),
// 		Resources: corev1.ResourceRequirements{
// 			Limits: corev1.ResourceList{
// 				corev1.ResourceMemory: limit,
// 			},
// 			Requests: corev1.ResourceList{
// 				corev1.ResourceMemory: limit,
// 			},
// 		},
// 		VolumeMounts:             createVolumeMounts(props, &plugin.MountSources, nil, plugin.Volumes),
// 		Env:                      append(envVars, commonEnvironmentVariables(props)...),
// 		TerminationMessagePolicy: corev1.TerminationMessageFallbackToLogsOnError,
// 	}
// 	return k8sContainer, nil
// }

// func exposedPortsToInts(exposedPorts []brokerModel.ExposedPort) []int {
// 	ports := []int{}
// 	for _, exposedPort := range exposedPorts {
// 		ports = append(ports, exposedPort.ExposedPort)
// 	}
// 	return ports
// }
