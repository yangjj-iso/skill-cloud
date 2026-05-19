package api

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/yangjj-iso/skill-cloud/server/internal/auth"
	"github.com/yangjj-iso/skill-cloud/server/internal/invocations"
	"github.com/yangjj-iso/skill-cloud/server/internal/models"
	"github.com/yangjj-iso/skill-cloud/server/internal/runtime"
)

func (s *Server) listSkills(c *gin.Context) {
	p, ok := auth.PrincipalFromContext(c.Request.Context())
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "no principal"})
		return
	}
	skills, err := s.registry.List(c.Request.Context(), p.OrgID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	redacted := make([]models.SkillManifest, len(skills))
	for i, sk := range skills {
		redacted[i] = sk.Redacted()
	}
	c.JSON(http.StatusOK, gin.H{"skills": redacted})
}

func (s *Server) getSkill(c *gin.Context) {
	p, ok := auth.PrincipalFromContext(c.Request.Context())
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "no principal"})
		return
	}
	ns := c.Param("namespace")
	name := c.Param("name")
	skill, found, err := s.registry.Get(c.Request.Context(), p.OrgID, ns, name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !found {
		c.JSON(http.StatusNotFound, gin.H{"error": "skill not found"})
		return
	}
	c.JSON(http.StatusOK, skill.Redacted())
}

// getSkillRuntime returns the runtime implementation details for an
// owned skill. The registry is already org-scoped, so a successful Get
// here proves the caller is in the owning org. This is the ONE endpoint
// that exposes runtime.image / entrypoint / url — deliberately separate
// so that anti-theft redaction on list/get can't be bypassed by accident.
func (s *Server) getSkillRuntime(c *gin.Context) {
	p, ok := auth.PrincipalFromContext(c.Request.Context())
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "no principal"})
		return
	}
	ns := c.Param("namespace")
	name := c.Param("name")
	skill, found, err := s.registry.Get(c.Request.Context(), p.OrgID, ns, name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !found {
		c.JSON(http.StatusNotFound, gin.H{"error": "skill not found"})
		return
	}
	c.JSON(http.StatusOK, skill.Runtime)
}

func (s *Server) createSkill(c *gin.Context) {
	p, ok := auth.PrincipalFromContext(c.Request.Context())
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "no principal"})
		return
	}
	var manifest models.SkillManifest
	if err := c.ShouldBindJSON(&manifest); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := manifest.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	skill, err := s.registry.Upsert(c.Request.Context(), p.OrgID, manifest)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, skill)
}

func (s *Server) invokeSkill(c *gin.Context) {
	p, ok := auth.PrincipalFromContext(c.Request.Context())
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "no principal"})
		return
	}
	ns := c.Param("namespace")
	name := c.Param("name")
	started := time.Now().UTC()

	skill, found, err := s.registry.Get(c.Request.Context(), p.OrgID, ns, name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !found {
		c.JSON(http.StatusNotFound, gin.H{"error": "skill not found"})
		return
	}

	rawBody, _ := io.ReadAll(c.Request.Body)
	var input map[string]any
	if len(rawBody) > 0 {
		if err := json.Unmarshal(rawBody, &input); err != nil {
			input = map[string]any{}
		}
	} else {
		input = map[string]any{}
	}

	result, _ := s.dispatcher.Run(c.Request.Context(), runtime.Request{Skill: skill, Input: input})

	response := gin.H{
		"skill":  skill.QualifiedName(),
		"status": result.Status,
		"output": result.Output,
	}
	if result.ErrorMessage != "" {
		response["error"] = result.ErrorMessage
	}

	s.recordInvocation(c, p, skill, started, result.Status, result.ErrorMessage, len(rawBody), result.OutputBytes)

	// Status code reflects whether the skill executed successfully. We
	// still write the body (including the error message) so callers can
	// see what went wrong without scraping logs.
	code := http.StatusOK
	switch result.Status {
	case runtime.StatusTimeout:
		code = http.StatusGatewayTimeout
	case runtime.StatusError:
		code = http.StatusBadGateway
	}
	c.JSON(code, response)
}

func (s *Server) getSkillStats(c *gin.Context) {
	p, ok := auth.PrincipalFromContext(c.Request.Context())
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "no principal"})
		return
	}
	ns := c.Param("namespace")
	name := c.Param("name")
	_, found, err := s.registry.Get(c.Request.Context(), p.OrgID, ns, name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !found {
		c.JSON(http.StatusNotFound, gin.H{"error": "skill not found"})
		return
	}
	stats, err := s.invocations.Stats(c.Request.Context(), p.OrgID, ns, name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, stats)
}

// recordInvocation writes an invocation row. Failures are logged but do
// not fail the request — losing audit fidelity is preferable to failing
// the user-facing call.
func (s *Server) recordInvocation(c *gin.Context, p auth.Principal, skill models.SkillManifest, started time.Time, status, errMsg string, inputBytes, outputBytes int) {
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
		CallerIP:     callerIPFromContext(c),
		UserAgent:    c.Request.UserAgent(),
		StartedAt:    started,
	}
	if err := s.invocations.Log(c.Request.Context(), entry); err != nil {
		log.Printf("invocations: log %s/%s: %v", skill.Namespace, skill.Name, err)
	}
}

// --- bootstrap auth handlers ---

type createOrgRequest struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
}

func (s *Server) createOrg(c *gin.Context) {
	var req createOrgRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Slug == "" || req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "slug and name are required"})
		return
	}
	id, err := s.auth.CreateOrg(c.Request.Context(), req.Slug, req.Name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": id, "slug": req.Slug, "name": req.Name})
}

type createUserRequest struct {
	OrgID string `json:"org_id"`
	Email string `json:"email"`
}

func (s *Server) createUser(c *gin.Context) {
	var req createUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	orgID, err := parseUUID(req.OrgID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid org_id: " + err.Error()})
		return
	}
	if req.Email == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "email is required"})
		return
	}
	id, err := s.auth.CreateUser(c.Request.Context(), orgID, req.Email)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": id, "org_id": orgID, "email": req.Email})
}

type createAPIKeyRequest struct {
	OrgID  string `json:"org_id"`
	UserID string `json:"user_id"`
	Name   string `json:"name"`
}

func (s *Server) createAPIKey(c *gin.Context) {
	var req createAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	orgID, err := parseUUID(req.OrgID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid org_id: " + err.Error()})
		return
	}
	userID, err := parseUUID(req.UserID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user_id: " + err.Error()})
		return
	}
	issued, err := s.auth.IssueAPIKey(c.Request.Context(), orgID, userID, req.Name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"id":     issued.ID,
		"prefix": issued.Prefix,
		// Plaintext is shown exactly once. Clients MUST store it now.
		"token": issued.Plaintext,
	})
}
