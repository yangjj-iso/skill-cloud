// Package mcp implements a minimal Model Context Protocol server endpoint
// that exposes every registered skill as an MCP tool.
//
// This is intentionally a stub of the MCP JSON-RPC contract — enough to
// satisfy `initialize`, `tools/list`, and `tools/call` so that
// MCP-capable clients can discover and call Skill Cloud skills. A full
// implementation (notifications, resources, prompts, streaming, etc.)
// will follow.
package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/yangjj-iso/skill-cloud/server/internal/auth"
	"github.com/yangjj-iso/skill-cloud/server/internal/invocations"
	"github.com/yangjj-iso/skill-cloud/server/internal/models"
	"github.com/yangjj-iso/skill-cloud/server/internal/registry"
	"github.com/yangjj-iso/skill-cloud/server/internal/runtime"
)

const protocolVersion = "2024-11-05"

type jsonRPCRequest struct {
	JSONRPC string         `json:"jsonrpc"`
	ID      any            `json:"id"`
	Method  string         `json:"method"`
	Params  map[string]any `json:"params"`
}

type jsonRPCResponse struct {
	JSONRPC string        `json:"jsonrpc"`
	ID      any           `json:"id"`
	Result  any           `json:"result,omitempty"`
	Error   *jsonRPCError `json:"error,omitempty"`
}

type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// CallerIPFunc resolves the caller IP from a gin.Context. The api
// package injects one that respects the SKILLCLOUD_TRUST_PROXY setting.
type CallerIPFunc func(c *gin.Context) string

// Options bundles MCP handler dependencies. All fields are optional;
// nil values disable the corresponding feature (invocation logging is
// skipped if Invocations is nil, runtime dispatch is replaced with a
// stub if Dispatcher is nil, etc.).
type Options struct {
	Invocations invocations.Store
	CallerIP    CallerIPFunc
	Dispatcher  *runtime.Dispatcher
}

// Handler returns a gin handler that implements the MCP JSON-RPC
// contract. `tools/list` is scoped by the authenticated principal's org
// and strips runtime details from every entry so that discovering
// clients (including org members) cannot trivially copy the underlying
// implementation. `tools/call` records each call to the invocations
// store when one is configured. Unit tests that don't go through the
// auth middleware may inject a Principal via auth.InjectPrincipal.
func Handler(reg registry.Registry, opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req jsonRPCRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, jsonRPCResponse{
				JSONRPC: "2.0",
				Error:   &jsonRPCError{Code: -32700, Message: "parse error"},
			})
			return
		}

		switch req.Method {
		case "initialize":
			c.JSON(http.StatusOK, jsonRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result: gin.H{
					"protocolVersion": protocolVersion,
					"serverInfo": gin.H{
						"name":    "skill-cloud",
						"version": "0.1.0",
					},
					"capabilities": gin.H{
						"tools": gin.H{},
					},
				},
			})

		case "tools/list":
			p, ok := auth.PrincipalFromContext(c.Request.Context())
			if !ok {
				c.JSON(http.StatusUnauthorized, jsonRPCResponse{
					JSONRPC: "2.0",
					ID:      req.ID,
					Error:   &jsonRPCError{Code: -32001, Message: "unauthenticated"},
				})
				return
			}
			skills, err := reg.List(c.Request.Context(), p.OrgID)
			if err != nil {
				c.JSON(http.StatusInternalServerError, jsonRPCResponse{
					JSONRPC: "2.0",
					ID:      req.ID,
					Error:   &jsonRPCError{Code: -32000, Message: err.Error()},
				})
				return
			}
			tools := make([]gin.H, 0, len(skills))
			for _, s := range skills {
				tools = append(tools, gin.H{
					"name":        s.QualifiedName(),
					"description": s.Description,
					"inputSchema": skillInputSchema(s.Inputs),
				})
			}
			c.JSON(http.StatusOK, jsonRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result:  gin.H{"tools": tools},
			})

		case "tools/call":
			handleToolsCall(c, req, reg, opts)

		default:
			c.JSON(http.StatusOK, jsonRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   &jsonRPCError{Code: -32601, Message: "method not found"},
			})
		}
	}
}

// handleToolsCall implements MCP `tools/call`. Real runtime dispatch is
// pending (M2), so the response is a stub. The skill lookup, however,
// is real: we validate the tool name, scope to the caller's org, and
// emit an invocation record so the audit trail is identical to the REST
// invoke path.
func handleToolsCall(c *gin.Context, req jsonRPCRequest, reg registry.Registry, opts Options) {
	started := time.Now().UTC()
	p, ok := auth.PrincipalFromContext(c.Request.Context())
	if !ok {
		c.JSON(http.StatusUnauthorized, jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &jsonRPCError{Code: -32001, Message: "unauthenticated"},
		})
		return
	}

	name, _ := req.Params["name"].(string)
	ns, skillName, ok := splitQualifiedName(name)
	if !ok {
		c.JSON(http.StatusBadRequest, jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &jsonRPCError{Code: -32602, Message: "invalid params: expected name=\"<namespace>/<name>\""},
		})
		return
	}

	skill, found, err := reg.Get(c.Request.Context(), p.OrgID, ns, skillName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &jsonRPCError{Code: -32000, Message: err.Error()},
		})
		return
	}
	if !found {
		c.JSON(http.StatusNotFound, jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &jsonRPCError{Code: -32004, Message: "tool not found"},
		})
		return
	}

	// Coerce arguments into the input map shape the dispatcher expects.
	inputBytes := 0
	input := map[string]any{}
	if raw, ok := req.Params["arguments"]; ok {
		if b, err := json.Marshal(raw); err == nil {
			inputBytes = len(b)
		}
		if m, ok := raw.(map[string]any); ok {
			input = m
		}
	}

	var runResult runtime.Result
	if opts.Dispatcher != nil {
		runResult, _ = opts.Dispatcher.Run(c.Request.Context(), runtime.Request{Skill: skill, Input: input})
	} else {
		runResult = runtime.Result{Status: runtime.StatusError, ErrorMessage: "dispatcher not configured"}
	}

	// MCP returns tool output as a list of content items. We serialise
	// the dispatcher's output map to JSON text — clients can re-parse
	// it if they need structured access.
	body, _ := json.Marshal(runResult.Output)
	if len(body) == 0 || string(body) == "null" {
		body = []byte("{}")
	}
	mcpResult := gin.H{
		"content": []gin.H{
			{
				"type": "text",
				"text": string(body),
			},
		},
	}
	if runResult.Status != runtime.StatusOK {
		mcpResult["isError"] = true
		mcpResult["content"] = []gin.H{
			{
				"type": "text",
				"text": runResult.ErrorMessage,
			},
		}
	}

	outputBytes := runResult.OutputBytes
	if outputBytes == 0 {
		outputBytes = len(body)
	}
	logInvocation(c, opts, p, skill, started, runResult.Status, runResult.ErrorMessage, inputBytes, outputBytes)

	c.JSON(http.StatusOK, jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  mcpResult,
	})
}

// splitQualifiedName parses a "<namespace>/<name>" identifier.
func splitQualifiedName(qualified string) (string, string, bool) {
	parts := strings.SplitN(qualified, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func logInvocation(c *gin.Context, opts Options, p auth.Principal, skill models.SkillManifest, started time.Time, status, errMsg string, inputBytes, outputBytes int) {
	if opts.Invocations == nil {
		return
	}
	ip := ""
	if opts.CallerIP != nil {
		ip = opts.CallerIP(c)
	}
	entry := invocations.Entry{
		OrgID:        p.OrgID,
		UserID:       p.UserID,
		APIKeyID:     p.APIKeyID,
		Namespace:    skill.Namespace,
		Name:         skill.Name,
		Version:      skill.Version,
		Status:       status,
		LatencyMS:    int(time.Since(started).Milliseconds()),
		InputBytes:   inputBytes,
		OutputBytes:  outputBytes,
		ErrorMessage: errMsg,
		CallerIP:     ip,
		UserAgent:    c.Request.UserAgent(),
		StartedAt:    started,
	}
	// Use a detached context so a client disconnect doesn't cancel the
	// audit write.
	_ = opts.Invocations.Log(context.Background(), entry)
}

func skillInputSchema(inputs map[string]models.IOField) map[string]any {
	properties := map[string]any{}
	required := []string{}
	for name, field := range inputs {
		prop := map[string]any{"type": field.Type}
		if field.Description != "" {
			prop["description"] = field.Description
		}
		if field.Default != nil {
			prop["default"] = field.Default
		}
		properties[name] = prop
		if field.Required {
			required = append(required, name)
		}
	}
	schema := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}
