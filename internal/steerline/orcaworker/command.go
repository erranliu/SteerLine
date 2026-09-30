// Package orcaworker builds fixed Orca worker commands without owning workflow state.
package orcaworker

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf16"
)

type TurnOptions struct {
	Executable      string
	Args            []string
	Sandbox         string
	Model           string
	ReasoningEffort string
	Profile         string
}

var threadID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func stableID(value string) bool {
	if len(value) != 32 {
		return false
	}
	for _, c := range value {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
func powershellLiteral(value string) string { return "'" + strings.ReplaceAll(value, "'", "''") + "'" }
func encodePowerShell(script string) string {
	words := utf16.Encode([]rune(script))
	data := make([]byte, len(words)*2)
	for i, word := range words {
		binary.LittleEndian.PutUint16(data[i*2:], word)
	}
	return base64.StdEncoding.EncodeToString(data)
}

const promptChunkBytes = 512

func promptRoot(executionID, turnID string) string {
	return "SteerLineWorkerFlow\\" + executionID + "\\" + turnID
}

func PromptChunks(prompt string) [][]byte {
	data := []byte(prompt)
	chunks := make([][]byte, 0, (len(data)+promptChunkBytes-1)/promptChunkBytes)
	for len(data) > 0 {
		count := min(len(data), promptChunkBytes)
		chunks = append(chunks, data[:count])
		data = data[count:]
	}
	return chunks
}

func ChunkCommand(executionID, turnID string, index int, chunk []byte) (string, string, error) {
	if !stableID(executionID) || !stableID(turnID) || index < 0 || len(chunk) == 0 || len(chunk) > promptChunkBytes {
		return "", "", errors.New("Orca prompt chunk has invalid identity or size")
	}
	digest := sha256.Sum256(chunk)
	sha := hex.EncodeToString(digest[:])
	marker := fmt.Sprintf("SLCHUNK:%s:%d:%s", turnID, index, sha)
	var script strings.Builder
	script.WriteString("$root = Join-Path ([IO.Path]::GetTempPath()) ")
	script.WriteString(powershellLiteral(promptRoot(executionID, turnID)))
	script.WriteString("\n[IO.Directory]::CreateDirectory($root) | Out-Null\n")
	script.WriteString("$path = Join-Path $root ")
	script.WriteString(powershellLiteral(fmt.Sprintf("prompt-%04d.bin", index)))
	script.WriteString("\n$data = [Convert]::FromBase64String('")
	script.WriteString(base64.StdEncoding.EncodeToString(chunk))
	script.WriteString("')\nif (-not [IO.File]::Exists($path)) { $file = [IO.File]::Open($path, [IO.FileMode]::CreateNew); try { $file.Write($data, 0, $data.Length) } finally { $file.Close() } }\n")
	script.WriteString("if ((Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLowerInvariant() -cne ")
	script.WriteString(powershellLiteral(sha))
	script.WriteString(") { throw 'fixed prompt chunk differs from the original request' }\nWrite-Output ")
	script.WriteString(powershellLiteral(marker))
	line := "powershell.exe -NoLogo -NoProfile -EncodedCommand " + encodePowerShell(script.String())
	if len(line) > 30000 {
		return "", "", errors.New("Orca prompt chunk exceeds the Windows command limit")
	}
	return line, marker, nil
}

func TurnCommand(opts TurnOptions, executionID, worktree, sessionID, turnID, prompt string, addDirs []string) (string, error) {
	if !stableID(executionID) || !stableID(turnID) || strings.TrimSpace(prompt) == "" || !filepath.IsAbs(worktree) || sessionID != "" && !threadID.MatchString(sessionID) {
		return "", errors.New("Orca worker turn lacks fixed prompt, workspace or session")
	}
	command := opts.Executable
	if command == "" {
		return "", errors.New("Orca worker Codex executable is not configured")
	}
	args := append([]string{}, opts.Args...)
	args = append(args, "exec", "--json", "--sandbox", opts.Sandbox, "-C", worktree)
	for _, dir := range addDirs {
		args = append(args, "--add-dir", dir)
	}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	if opts.ReasoningEffort != "" {
		args = append(args, "--config", "model_reasoning_effort="+opts.ReasoningEffort)
	}
	if opts.Profile != "" {
		args = append(args, "--profile", opts.Profile)
	}
	if sessionID != "" {
		args = append(args, "resume", sessionID)
	}
	args = append(args, "-")
	var script strings.Builder
	script.WriteString("$OutputEncoding = [Text.UTF8Encoding]::new($false)\n")
	script.WriteString("$root = Join-Path ([IO.Path]::GetTempPath()) ")
	script.WriteString(powershellLiteral(promptRoot(executionID, turnID)))
	script.WriteString("\n$stream = [IO.MemoryStream]::new()\n")
	script.WriteString("for ($index = 0; $index -lt ")
	script.WriteString(strconv.Itoa(len(PromptChunks(prompt))))
	script.WriteString("; $index++) { $path = Join-Path $root ('prompt-{0:D4}.bin' -f $index); $part = [IO.File]::ReadAllBytes($path); $stream.Write($part, 0, $part.Length) }\n")
	script.WriteString("$bytes = $stream.ToArray(); $sha = [Security.Cryptography.SHA256]::Create(); try { $digest = ([BitConverter]::ToString($sha.ComputeHash($bytes))).Replace('-', '').ToLowerInvariant() } finally { $sha.Dispose() }\n")
	fullDigest := sha256.Sum256([]byte(prompt))
	script.WriteString("if ($digest -cne ")
	script.WriteString(powershellLiteral(hex.EncodeToString(fullDigest[:])))
	script.WriteString(") { throw 'fixed worker prompt digest differs from the saved message' }\n")
	script.WriteString("$lock = Join-Path $root 'launch.lock'; try { $file = [IO.File]::Open($lock, [IO.FileMode]::CreateNew); try { $saved = [Text.Encoding]::UTF8.GetBytes($digest); $file.Write($saved, 0, $saved.Length) } finally { $file.Close() } } catch [IO.IOException] { if ([IO.File]::Exists($lock) -and [IO.File]::ReadAllText($lock) -ceq $digest) { Write-Output 'SLALREADY:")
	script.WriteString(turnID)
	script.WriteString("'; exit 0 }; throw }\n")
	script.WriteString("$prompt = [Text.Encoding]::UTF8.GetString($bytes)\n")
	script.WriteString("$prompt | & ")
	script.WriteString(powershellLiteral(command))
	for _, arg := range args {
		script.WriteByte(' ')
		script.WriteString(powershellLiteral(arg))
	}
	script.WriteString(" | ForEach-Object { Write-Output ('SLJSON:")
	script.WriteString(turnID)
	script.WriteString(":' + [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($_))) }\n")
	script.WriteString("Write-Output ('SLEND:")
	script.WriteString(turnID)
	script.WriteString(":' + $LASTEXITCODE)\n")
	encoded := encodePowerShell(script.String())
	line := "powershell.exe -NoLogo -NoProfile -EncodedCommand " + encoded
	if len(line) > 30000 {
		return "", fmt.Errorf("worker launch command exceeds Windows command limit")
	}
	return line, nil
}
