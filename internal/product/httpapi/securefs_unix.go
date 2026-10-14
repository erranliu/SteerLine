//go:build !windows

package httpapi

import fs "github.com/erranliu/SteerLine/internal/product/httpfs"

var createPrivateTemp = fs.CreatePrivateTemp
var ensurePrivateDirectory = fs.EnsurePrivateDirectory
var verifyPrivateDirectory = fs.VerifyPrivateDirectory
var securePrivateFile = fs.SecurePrivateFile
var verifyPrivateFile = fs.VerifyPrivateFile
var privatePath = fs.PrivatePath
