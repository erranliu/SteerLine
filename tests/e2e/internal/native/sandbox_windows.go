package native

import (
	"encoding/base64"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

var sandboxLogon = windows.NewLazySystemDLL("advapi32.dll").NewProc("LogonUserW")

func validateSandboxAccounts(users sandboxUsers) error {
	for _, account := range []sandboxAccount{users.Offline, users.Online} {
		if err := validateSandboxAccount(account); err != nil {
			return fmt.Errorf("existing sandbox account %s is unusable: %w; initialize Codex sandbox once outside the test runner", account.Username, err)
		}
	}
	return nil
}

func validateSandboxAccount(account sandboxAccount) error {
	encrypted, err := base64.StdEncoding.DecodeString(account.Password)
	if err != nil || len(encrypted) == 0 {
		return fmt.Errorf("invalid encrypted credential")
	}
	in := windows.DataBlob{Size: uint32(len(encrypted)), Data: &encrypted[0]}
	var out windows.DataBlob
	// CRYPTPROTECT_UI_FORBIDDEN: credential decoding must never show a dialog.
	if err := windows.CryptUnprotectData(&in, nil, nil, 0, nil, 1, &out); err != nil {
		return fmt.Errorf("decode existing credential: %w", err)
	}
	decrypted := unsafe.Slice(out.Data, int(out.Size))
	defer func() {
		clear(decrypted)
		windows.LocalFree(windows.Handle(uintptr(unsafe.Pointer(out.Data))))
	}()
	password, err := windows.UTF16FromString(string(decrypted))
	if err != nil {
		return fmt.Errorf("invalid decoded credential")
	}
	defer clear(password)
	username, err := windows.UTF16PtrFromString(account.Username)
	if err != nil {
		return err
	}
	domain, _ := windows.UTF16PtrFromString(".")
	var token windows.Handle
	// Same interactive logon type used by the native Codex launcher. This
	// authenticates an existing account; it does not provision or elevate it.
	ok, _, callErr := sandboxLogon.Call(uintptr(unsafe.Pointer(username)), uintptr(unsafe.Pointer(domain)), uintptr(unsafe.Pointer(&password[0])), 2, 0, uintptr(unsafe.Pointer(&token)))
	if ok == 0 {
		return fmt.Errorf("existing account logon failed: %w", callErr)
	}
	return windows.CloseHandle(token)
}
