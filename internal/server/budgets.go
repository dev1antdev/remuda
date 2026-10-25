package server

import (
	"net/http"

	"github.com/dev1antdev/remuda/internal/budget"
)

// Budgets: the spending limits with what their periods have cost so far, the list replaced whole.

func (s *Server) budgetRoutes(api *http.ServeMux) {
	api.HandleFunc("GET /api/budgets", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, s.Budgets.Status()) })
	api.HandleFunc("PUT /api/budgets", s.handleBudgetsPut)
}

func (s *Server) handleBudgetsPut(w http.ResponseWriter, r *http.Request) {
	var list []budget.Budget
	if !decode(w, r, &list) {
		return
	}
	out, err := s.Budgets.Replace(list)
	if err != nil {
		httpErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, out)
}
