package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type Organization struct {
	ID     string `gorm:"primaryKey"`
	Name   string
	Config string `gorm:"type:json"`

	Kindergartens []Kindergarten `gorm:"-:all"`
	// Users []User `gorm:"many2many:organization_users;"`
}

type User struct {
	ID       uint   `gorm:"primaryKey"`
	Username string `gorm:"uniqueIndex"`
	Password string
	Role     string

	// Organizations []*Organization `gorm:"many2many:organization_users;"`
}

var centralDB *gorm.DB

func initCentralDB() {
	var err error
	centralDB, err = gorm.Open(sqlite.Open("central.db"), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to connect to central database: %v", err)
	}

	centralDB.AutoMigrate(&Organization{})
}

func getTenantDB(dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	db.AutoMigrate(&User{})
	return db, nil
}

func TenantMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenantID := r.Header.Get("X-Tenant-ID")
		if tenantID == "" {
			http.Error(w, "tenant ID is required", http.StatusBadRequest)
			return
		}

		var organization Organization
		if err := centralDB.Where("id = ?", tenantID).First(&organization).Error; err != nil {
			http.Error(w, "invalid tenant ID", http.StatusBadRequest)
			return
		}

		db, err := getTenantDB(organization.Config)
		if err != nil {
			http.Error(w, "failed to connect to tenant database", http.StatusInternalServerError)
			return
		}

		ctx := context.WithValue(r.Context(), "tenantDB", db)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func main() {
	initCentralDB()

	r := chi.NewRouter()
	r.Use(middleware.Logger)

	// Organization CRUD
	r.Route("/organizations", func(r chi.Router) {
		r.Post("/", createOrganization)
		r.Get("/", listOrganizations)
		r.Get("/{id}", getOrganization)
		r.Put("/{id}", updateOrganization)
		r.Delete("/{id}", deleteOrganization)
	})

	// User CRUD
	r.Route("/users", func(r chi.Router) {
		r.Post("/", createUser)
		r.Get("/", listUsers)
		r.Get("/{id}", getUser)
		r.Put("/{id}", updateUser)
		r.Delete("/{id}", deleteUser)
	})

	r.Route("/kindergartens", func(r chi.Router) {
		r.Use(TenantMiddleware)
		r.Get("/", listKindergartens)
	})

	log.Println("Starting server on :8080")

	err := http.ListenAndServe(":8080", r)

	if err != nil {
		panic(fmt.Sprintf("cannot start server: %s", err))
	}
}

func createOrganization(w http.ResponseWriter, r *http.Request) {
	var org Organization
	if err := json.NewDecoder(r.Body).Decode(&org); err != nil {
		http.Error(w, "invalid input", http.StatusBadRequest)
		return
	}
	if err := centralDB.Create(&org).Error; err != nil {
		http.Error(w, "could not create organization", http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(org)
}

func listOrganizations(w http.ResponseWriter, r *http.Request) {
	var organizations []Organization
	if err := centralDB.Find(&organizations).Error; err != nil {
		http.Error(w, "could not list organizations", http.StatusInternalServerError)
		return
	}

	for i, org := range organizations {
		tenantDB, err := getTenantDB(org.Config)
		if err != nil {
			http.Error(w, "failed to connect to tenant database", http.StatusInternalServerError)
			return
		}

		var kindergartens []Kindergarten
		if err := tenantDB.Find(&kindergartens).Error; err != nil {
			http.Error(w, "could not list kindergartens", http.StatusInternalServerError)
			return
		}

		org.Kindergartens = kindergartens
		organizations[i] = org

	}
	json.NewEncoder(w).Encode(organizations)
}

func getOrganization(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var organization Organization
	if err := centralDB.First(&organization, "id = ?", id).Error; err != nil {
		http.Error(w, "organization not found", http.StatusNotFound)
		return
	}
	json.NewEncoder(w).Encode(organization)
}

func updateOrganization(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var organization Organization
	if err := centralDB.First(&organization, "id = ?", id).Error; err != nil {
		http.Error(w, "organization not found", http.StatusNotFound)
		return
	}
	if err := json.NewDecoder(r.Body).Decode(&organization); err != nil {
		http.Error(w, "invalid input", http.StatusBadRequest)
		return
	}
	if err := centralDB.Save(&organization).Error; err != nil {
		http.Error(w, "could not update organization", http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(organization)
}

func deleteOrganization(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := centralDB.Delete(&Organization{}, "id = ?", id).Error; err != nil {
		http.Error(w, "could not delete organization", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func createUser(w http.ResponseWriter, r *http.Request) {
	var user User
	if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
		http.Error(w, "invalid input", http.StatusBadRequest)
		return
	}
	if err := centralDB.Create(&user).Error; err != nil {
		http.Error(w, "could not create user", http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(user)
}

func listUsers(w http.ResponseWriter, r *http.Request) {
	var users []User
	if err := centralDB.Find(&users).Error; err != nil {
		http.Error(w, "could not list users", http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(users)
}

func getUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var user User
	if err := centralDB.First(&user, "id = ?", id).Error; err != nil {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}
	json.NewEncoder(w).Encode(user)
}

func updateUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var user User
	if err := centralDB.First(&user, "id = ?", id).Error; err != nil {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}
	if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
		http.Error(w, "invalid input", http.StatusBadRequest)
		return
	}
	if err := centralDB.Save(&user).Error; err != nil {
		http.Error(w, "could not update user", http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(user)
}

func deleteUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := centralDB.Delete(&User{}, "id = ?", id).Error; err != nil {
		http.Error(w, "could not delete user", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type Kindergarten struct {
	ID   string `gorm:"primaryKey"`
	Name string
}

func listKindergartens(w http.ResponseWriter, r *http.Request) {

	tenantDB := r.Context().Value("tenantDB").(*gorm.DB)
	tenantDB.AutoMigrate(&Kindergarten{})

	tenantDB.Create(&Kindergarten{ID: "1", Name: "Kindergarten 1"})
	tenantDB.Create(&Kindergarten{ID: "2", Name: "Kindergarten 2"})

	var kindergartens []Kindergarten
	if err := tenantDB.Find(&kindergartens).Error; err != nil {
		http.Error(w, "could not list kindergartens", http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(kindergartens)
}
