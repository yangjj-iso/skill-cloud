package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/yangjj-iso/skill-cloud/server/internal/auth"
	"github.com/yangjj-iso/skill-cloud/server/internal/models"
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
	c.JSON(http.StatusOK, gin.H{"skills": skills})
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
	c.JSON(http.StatusOK, skill)
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
	skill, found, err := s.registry.Get(c.Request.Context(), p.OrgID, ns, name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !found {
		c.JSON(http.StatusNotFound, gin.H{"error": "skill not found"})
		return
	}

	var input map[string]any
	if err := c.ShouldBindJSON(&input); err != nil {
		// Empty body is allowed.
		input = map[string]any{}
	}

	// Real runtime dispatch is not yet implemented — return a stub
	// response so the SDK / MCP integration can be developed in parallel.
	c.JSON(http.StatusOK, gin.H{
		"skill":  skill.QualifiedName(),
		"input":  input,
		"output": gin.H{"message": "stub invocation — runtime dispatch not yet implemented"},
		"status": "ok",
	})
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
