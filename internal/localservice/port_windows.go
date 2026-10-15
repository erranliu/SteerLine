//go:build windows

package localservice

import (
	"encoding/binary"
	"fmt"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// portOwner observes only the conflicting loopback listener. It never stops
// another home or treats that listener as this application's Web endpoint.
func portOwner(address string) string {
	_, rawPort, err := net.SplitHostPort(address)
	if err != nil {
		return ""
	}
	port, err := strconv.Atoi(rawPort)
	if err != nil {
		return ""
	}
	query := windows.NewLazySystemDLL("iphlpapi.dll").NewProc("GetExtendedTcpTable")
	var size uint32
	query.Call(0, uintptr(unsafe.Pointer(&size)), 0, windows.AF_INET, 3, 0)
	if size < 4 || size > 4<<20 {
		return ""
	}
	data := make([]byte, size)
	result, _, _ := query.Call(uintptr(unsafe.Pointer(&data[0])), uintptr(unsafe.Pointer(&size)), 0, windows.AF_INET, 3, 0)
	if result != 0 {
		return ""
	}
	count := int(binary.LittleEndian.Uint32(data[:4]))
	if count > (len(data)-4)/24 {
		return ""
	}
	for i := range count {
		row := data[4+i*24 : 4+(i+1)*24]
		if int(binary.BigEndian.Uint16(row[8:10])) != port {
			continue
		}
		ip := row[4:8]
		if !(ip[0] == 0 && ip[1] == 0 && ip[2] == 0 && ip[3] == 0 || ip[0] == 127 && ip[1] == 0 && ip[2] == 0 && ip[3] == 1) {
			continue
		}
		pid := binary.LittleEndian.Uint32(row[20:24])
		detail := fmt.Sprintf("owner pid=%d", pid)
		handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
		if err != nil {
			return detail
		}
		path := make([]uint16, 32768)
		length := uint32(len(path))
		err = windows.QueryFullProcessImageName(handle, 0, &path[0], &length)
		windows.CloseHandle(handle)
		if err != nil {
			return detail
		}
		exe := windows.UTF16ToString(path[:length])
		detail += " executable=" + exe
		if strings.EqualFold(filepath.Base(exe), "steerline-web.exe") {
			detail += " home=" + filepath.Dir(filepath.Dir(exe))
		}
		return detail
	}
	return ""
}
