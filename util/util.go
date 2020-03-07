package util

import (
	"bytes"
	"os/exec"
	"strings"
)

// GetContainerName get container name from docker process.
func GetContainerName() string {
	out, err := execShell(`cat /proc/self/cgroup | grep "cpu:/"`)
	if err != nil {
		return ""
	}
	items := strings.Split(out, "/")
	if l := len(items); l > 0 {
		return items[l-1]
	}
	return ""
}

func execShell(s string) (string, error) {
	// cmd for alpine docker
	cmd := exec.Command("/bin/sh", "-c", s)

	var out bytes.Buffer
	cmd.Stdout = &out

	err := cmd.Run()
	return out.String(), err
}

// GenUUID generate a 64bit UUID.
//
// Detail format:
// 0            0........0			0....................0	0.............................0
// 1bit sign    10bit serviceID     22bit containerID       31bit sequenceID
func GenUUID(serviceID int32, containerID int32, sequenceID int32) int64 {
	var uuid uint64
	uuid |= uint64(serviceID) << 53
	uuid |= uint64(containerID) << 31
	uuid |= uint64(sequenceID)
	return int64(uuid)
}

// Min return the smallest number of x, y
func Min(x, y int) int {
	if x > y {
		return y
	}
	return x
}

// Max return the biggest number of x, y
func Max(x, y int) int {
	if x > y {
		return x
	}
	return y
}
