package runtime

import (
	"fmt"
)

func GetContainerStatusByName(pod *Pod, name string) (ContainerStatus, error) {
	for _, container := range pod.Status.Containers {
		c := container
		if container.Name == name {
			return c, nil
		}
	}

	return ContainerStatus{}, fmt.Errorf("no such container %s found", name)
}

func GetContainerSpecByName(pod *Pod, name string) (ContainerSpec, error) {
	for _, container := range pod.Spec.Containers {
		c := container
		if container.Name == name {
			return c, nil
		}
	}

	return ContainerSpec{}, fmt.Errorf("no such container %s found", name)
}

func GetContainerStatusByID(pod *Pod, id string) (ContainerStatus, error) {
	for _, container := range pod.Status.Containers {
		c := container
		if container.ContainerID == id {
			return c, nil
		}
	}

	return ContainerStatus{}, fmt.Errorf("no such container %s found", id)
}
