package util

import (
	"bytes"
	"os/exec"
	"strings"
)

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

// generate uuid, format:
//
// 0		0........0			0....................0	0.............................0
// 1-bit	10bit service id	22bit container id		31bit sequence
func GenUUID(serviceID int32, containerID int32, sequenceID int32) int64 {
	var uuid uint64
	uuid |= uint64(serviceID) << 53
	uuid |= uint64(containerID) << 31
	uuid |= uint64(sequenceID)
	return int64(uuid)
}

func Min(x, y int) int {
	if x > y {
		return y
	}
	return x
}

func Max(x, y int) int {
	if x > y {
		return x
	}
	return y
}
