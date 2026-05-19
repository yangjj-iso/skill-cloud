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
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/yangjj-iso/skill-cloud/server/internal/auth"
	"github.com/yangjj-iso/skill-cloud/server/internal/models"
	"github.com/yangjj-iso/skill-cloud/server/internal/registry"
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

// Handler returns a gin handler that implements the MCP JSON-RPC contract.
// `tools/list` is scoped by the authenticated principal's org so each
// tenant only sees its own skills. Unit tests that don't go through the
// auth middleware may inject a Principal via auth.InjectPrincipal.
func Handler(reg registry.Registry) gin.HandlerFunc {
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
			// MVP stub: echo the call. A future change will route this
			// through the runtime dispatcher.
			c.JSON(http.StatusOK, jsonRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result: gin.H{
					"content": []gin.H{
						{
							"type": "text",
							"text": "stub invocation — runtime dispatch not yet implemented",
						},
					},
				},
			})

		default:
			c.JSON(http.StatusOK, jsonRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   &jsonRPCError{Code: -32601, Message: "method not found"},
			})
		}
	}
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
