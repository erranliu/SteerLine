package httpfs

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	. "github.com/erranliu/SteerLine/internal/product/httpcontract"
	"io"
	"strings"
)

const maxDiscoveryBytes = 64 << 10

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	_, err := rand.Read(b)
	return hex.EncodeToString(b), err
}

func ReadDiscoveryFile(path string) (Discovery, error) {
	var discovery Discovery
	if err := VerifyPrivateFile(path); err != nil {
		return discovery, err
	}
	file, err := openDiscoveryReader(path)
	if err != nil {
		return discovery, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxDiscoveryBytes+1))
	if err != nil {
		return discovery, err
	}
	if len(data) == 0 || len(data) > maxDiscoveryBytes {
		return discovery, errors.New("local API discovery file is empty or too large")
	}
	if err = decodeStrict(data, &discovery); err != nil {
		return discovery, err
	}
	failedWithoutListener := !discovery.Ready && discovery.Status == "failed" && discovery.Address == "" && discovery.CredentialPath == ""
	if discovery.Version != 1 || discovery.Protocol != Protocol || discovery.ProjectID != ProjectID(discovery.RepositoryIdentity) ||
		(!failedWithoutListener && (discovery.Address == "" || discovery.CredentialPath == "")) || discovery.LeaseID == "" {
		return Discovery{}, errors.New("local API discovery has an unsupported or incomplete identity")
	}
	return discovery, nil
}

func decodeStrict(data []byte, target any) error {
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if decoder.Decode(new(any)) != io.EOF {
		return errors.New("JSON must contain one value")
	}
	return nil
}
