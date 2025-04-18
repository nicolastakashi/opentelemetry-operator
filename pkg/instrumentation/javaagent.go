// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package instrumentation

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"

	"github.com/open-telemetry/opentelemetry-operator/apis/v1alpha1"
)

const (
	envJavaToolsOptions   = "JAVA_TOOL_OPTIONS"
	javaAgent             = " -javaagent:/otel-auto-instrumentation-java/javaagent.jar"
	javaInitContainerName = initContainerName + "-java"
	javaVolumeName        = volumeName + "-java"
	javaInstrMountPath    = "/otel-auto-instrumentation-java"
)

func injectJavaagent(javaSpec v1alpha1.Java, pod corev1.Pod, index int, instSpec v1alpha1.InstrumentationSpec) (corev1.Pod, error) {
	volume := instrVolume(javaSpec.VolumeClaimTemplate, javaVolumeName, javaSpec.VolumeSizeLimit)

	// caller checks if there is at least one container.
	container := &pod.Spec.Containers[index]

	err := validateContainerEnv(container.Env, envJavaToolsOptions)
	if err != nil {
		return pod, err
	}

	// inject Java instrumentation spec env vars.
	container.Env = appendIfNotSet(container.Env, javaSpec.Env...)

	// Create unique mount path for this container
	containerMountPath := fmt.Sprintf("%s-%s", javaInstrMountPath, container.Name)

	javaJVMArgument := fmt.Sprintf(" -javaagent:%s/javaagent.jar", containerMountPath)
	if len(javaSpec.Extensions) > 0 {
		javaJVMArgument = javaJVMArgument + fmt.Sprintf(" -Dotel.javaagent.extensions=%s/extensions", containerMountPath)
	}

	idx := getIndexOfEnv(container.Env, envJavaToolsOptions)
	if idx == -1 {
		container.Env = append(container.Env, corev1.EnvVar{
			Name:  envJavaToolsOptions,
			Value: javaJVMArgument,
		})
	} else {
		container.Env[idx].Value = container.Env[idx].Value + javaJVMArgument
	}

	container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
		Name:      volume.Name,
		MountPath: containerMountPath,
	})

	// We just inject Volumes and init containers for the first processed container.
	if isInitContainerMissing(pod, javaInitContainerName) {
		pod.Spec.Volumes = append(pod.Spec.Volumes, volume)
		pod.Spec.InitContainers = append(pod.Spec.InitContainers, corev1.Container{
			Name:      javaInitContainerName,
			Image:     javaSpec.Image,
			Command:   []string{"cp", "/javaagent.jar", javaInstrMountPath + "/javaagent.jar"},
			Resources: javaSpec.Resources,
			VolumeMounts: []corev1.VolumeMount{{
				Name:      volume.Name,
				MountPath: javaInstrMountPath,
			}},
			ImagePullPolicy: instSpec.ImagePullPolicy,
		})

		for i, extension := range javaSpec.Extensions {
			pod.Spec.InitContainers = append(pod.Spec.InitContainers, corev1.Container{
				Name:      initContainerName + fmt.Sprintf("-extension-%d", i),
				Image:     extension.Image,
				Command:   []string{"cp", "-r", extension.Dir + "/.", javaInstrMountPath + "/extensions"},
				Resources: javaSpec.Resources,
				VolumeMounts: []corev1.VolumeMount{{
					Name:      volume.Name,
					MountPath: javaInstrMountPath,
				}},
			})
		}
	}
	return pod, err
}
