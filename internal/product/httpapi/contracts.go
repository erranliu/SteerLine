package httpapi

import (
	c "github.com/erranliu/SteerLine/internal/product/httpcontract"
	"regexp"
)

const Protocol = c.Protocol

func ProjectID(identity string) string { return c.ProjectID(identity) }

var requestIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

type Identity = c.Identity
type ProcessIdentity = c.ProcessIdentity
type Discovery = c.Discovery
type ErrorResponse = c.ErrorResponse
type CommandEnvelope = c.CommandEnvelope
type CommandResponse = c.CommandResponse
type ActivityPage = c.ActivityPage
type ConversationPage = c.ConversationPage
type ProjectPresentation = c.ProjectPresentation
type FinalReply = c.FinalReply
type FinalBody = c.FinalBody
