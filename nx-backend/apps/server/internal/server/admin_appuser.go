package server

import "net/http"

func (s *Server) adminAppUserByID(w http.ResponseWriter, r *http.Request) {
	permission := "Customer:App:List"
	if r.Method == http.MethodPut || r.Method == http.MethodPatch {
		permission = "Customer:App:Write"
	}
	s.requirePermission(permission, s.appUsers.HandleAppUserByID)(w, r)
}
