package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/yangjj-iso/skill-cloud/server/internal/models"
)

func (s *Server) listSkills(c *gin.Context) {
	skills := s.registry.List()
	c.JSON(http.StatusOK, gin.H{"skills": skills})
}

func (s *Server) getSkill(c *gin.Context) {
	ns := c.Param("namespace")
	name := c.Param("name")
	skill, ok := s.registry.Get(ns, name)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "skill not found"})
		return
	}
	c.JSON(http.StatusOK, skill)
}

func (s *Server) createSkill(c *gin.Context) {
	var manifest models.SkillManifest
	if err := c.ShouldBindJSON(&manifest); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := manifest.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	skill := s.registry.Upsert(manifest)
	c.JSON(http.StatusCreated, skill)
}

func (s *Server) invokeSkill(c *gin.Context) {
	ns := c.Param("namespace")
	name := c.Param("name")
	skill, ok := s.registry.Get(ns, name)
	if !ok {
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
