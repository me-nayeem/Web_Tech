package main

import (
	"net/http"

	"school-management/backend/internal/db"
	"school-management/backend/internal/handlers"
	"school-management/backend/internal/httpx"
	"school-management/backend/internal/middleware"
)

func notFound(w http.ResponseWriter, _ *http.Request) {
	httpx.Error(w, http.StatusNotFound, httpx.CodeNotFound, "route not found")
}


func adminRoutes(h *handlers.Handlers) http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("GET /api/admin/users", h.AdminUsers)
	m.HandleFunc("PATCH /api/admin/users/{id}/role", h.AdminSetRole)
	m.HandleFunc("PATCH /api/admin/students/{id}/class", h.AdminSetStudentClass)

	m.HandleFunc("POST /api/admin/classes", h.AdminCreateClass)
	m.HandleFunc("DELETE /api/admin/classes/{id}", h.AdminDeleteClass)
	m.HandleFunc("POST /api/admin/subjects", h.AdminCreateSubject)
	m.HandleFunc("DELETE /api/admin/subjects/{id}", h.AdminDeleteSubject)

	m.HandleFunc("GET /api/admin/assignments", h.AdminAssignments)
	m.HandleFunc("POST /api/admin/assignments", h.AdminCreateAssignment)
	m.HandleFunc("DELETE /api/admin/assignments/{id}", h.AdminDeleteAssignment)

	m.HandleFunc("GET /api/admin/timetable", h.AdminTimetable)
	m.HandleFunc("POST /api/admin/timetable", h.AdminCreateSlot)
	m.HandleFunc("DELETE /api/admin/timetable/{id}", h.AdminDeleteSlot)

	m.HandleFunc("GET /api/admin/fees", h.AdminFees)
	m.HandleFunc("PUT /api/admin/classes/{id}/fee", h.AdminSetFee)

	m.HandleFunc("GET /api/admin/results", h.AdminResults)
	m.HandleFunc("POST /api/admin/classes/{id}/results/publish", h.AdminPublishResults)
	m.HandleFunc("GET /api/admin/reports", h.AdminReports)

	m.HandleFunc("/api/admin/", notFound)
	return m
}


func teacherRoutes(h *handlers.Handlers) http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("GET /api/teacher/assignments", h.TeacherAssignments)
	m.HandleFunc("GET /api/teacher/timetable", h.TeacherTimetable)
	m.HandleFunc("GET /api/teacher/assignments/{id}/students", h.TeacherStudents)
	m.HandleFunc("PUT /api/teacher/assignments/{id}/marks", h.TeacherSaveMarks)
	m.HandleFunc("/api/teacher/", notFound)
	return m
}

func studentRoutes(h *handlers.Handlers) http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("GET /api/student/profile", h.StudentProfile)
	m.HandleFunc("GET /api/student/timetable", h.StudentTimetable)
	m.HandleFunc("GET /api/student/fee", h.StudentFee)
	m.HandleFunc("GET /api/student/results", h.StudentResults)
	m.HandleFunc("/api/student/", notFound)
	return m
}

func newRouter(frontendURL string, verifier middleware.TokenVerifier, store middleware.UserStore, h *handlers.Handlers) http.Handler {
	api := http.NewServeMux()
	api.HandleFunc("GET /api/me", h.Me)
	api.HandleFunc("POST /api/onboarding", h.Onboarding)
	api.HandleFunc("GET /api/classes", h.Classes)
	api.HandleFunc("GET /api/subjects", h.Subjects)

	
	api.Handle("/api/admin/", middleware.RequireRole(db.RoleAdmin)(adminRoutes(h)))
	api.Handle("/api/teacher/", middleware.RequireRole(db.RoleTeacher)(teacherRoutes(h)))
	api.Handle("/api/student/", middleware.RequireRole(db.RoleStudent)(studentRoutes(h)))
	api.HandleFunc("/api/", notFound)

	protected := middleware.Chain(api,
		middleware.Authenticate(verifier, store),
		middleware.RequireOnboarding,
	)

	root := http.NewServeMux()
	root.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok"))
	})
	root.Handle("/api/", protected)
	root.HandleFunc("/", notFound)

	return middleware.Chain(root,
		middleware.Recover,
		middleware.Logger,
		middleware.SecurityHeaders,
		middleware.CORS(frontendURL),
	)
}