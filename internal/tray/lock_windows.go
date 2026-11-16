package tray

import (
	"golang.org/x/sys/windows"
)

var instanceMutex windows.Handle

// singleInstance devuelve false si ya hay otra bandeja corriendo en esta sesion.
func singleInstance() bool {
	name, _ := windows.UTF16PtrFromString(`Local\claude-monitor-tray`)
	h, err := windows.CreateMutex(nil, true, name)
	if err == windows.ERROR_ALREADY_EXISTS {
		if h != 0 {
			windows.CloseHandle(h)
		}
		return false
	}
	instanceMutex = h // se mantiene abierto mientras viva el proceso
	return err == nil
}
