package server

import (
	"net/http"

	"github.com/dev1antdev/remuda/internal/recipes"
)

// Recipes: the saved ways to start an agent, the list replaced whole.

func (s *Server) recipeRoutes(api *http.ServeMux) {
	api.HandleFunc("GET /api/recipes", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, s.Recipes.List()) })
	api.HandleFunc("PUT /api/recipes", s.handleRecipesPut)
}

func (s *Server) handleRecipesPut(w http.ResponseWriter, r *http.Request) {
	var list []recipes.Recipe
	if !decode(w, r, &list) {
		return
	}
	out, err := s.Recipes.Replace(list)
	if err != nil {
		httpErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, out)
}
