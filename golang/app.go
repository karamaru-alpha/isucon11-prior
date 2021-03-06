package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/jmoiron/sqlx"
)

var (
	publicDir string
	fs        http.Handler
)

type User struct {
	ID        string    `db:"id" json:"id"`
	Email     string    `db:"email" json:"email"`
	Nickname  string    `db:"nickname" json:"nickname"`
	Staff     bool      `db:"staff" json:"staff"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

type Schedule struct {
	ID           string         `db:"id" json:"id"`
	Title        string         `db:"title" json:"title"`
	Capacity     int            `db:"capacity" json:"capacity"`
	Reserved     int            `db:"reserved" json:"reserved"`
	Reservations []*Reservation `db:"reservations" json:"reservations"`
	CreatedAt    time.Time      `db:"created_at" json:"created_at"`
}

type Reservation struct {
	ID         string    `db:"id" json:"id"`
	ScheduleID string    `db:"schedule_id" json:"schedule_id"`
	UserID     string    `db:"user_id" json:"user_id"`
	User       *User     `db:"user" json:"user"`
	CreatedAt  time.Time `db:"created_at" json:"created_at"`
}

func getCurrentUser(r *http.Request) *User {
	uidCookie, err := r.Cookie("user_id")
	if err != nil || uidCookie == nil {
		return nil
	}
	row := db.QueryRowxContext(r.Context(), "SELECT * FROM `users` WHERE `id` = ? LIMIT 1", uidCookie.Value)
	user := &User{}
	if err := row.StructScan(user); err != nil {
		return nil
	}
	return user
}

func requiredLogin(w http.ResponseWriter, r *http.Request, u *User) bool {
	if u != nil {
		return true
	}
	sendErrorJSON(w, fmt.Errorf("login required"), 401)
	return false
}

func requiredStaffLogin(w http.ResponseWriter, r *http.Request) bool {
	if getCurrentUser(r) != nil && getCurrentUser(r).Staff {
		return true
	}
	sendErrorJSON(w, fmt.Errorf("login required"), 401)
	return false
}

func getReservations(r *http.Request, s *Schedule) error {
	// rows, err := db.QueryxContext(r.Context(), "SELECT * FROM `reservations` WHERE `schedule_id` = ?", s.ID)
	// if err != nil {
	// 	return err
	// }

	// defer rows.Close()

	// reserved := 0
	// s.Reservations = []*Reservation{}
	// for rows.Next() {
	// 	reservation := &Reservation{}
	// 	if err := rows.StructScan(reservation); err != nil {
	// 		return err
	// 	}
	// 	reservation.User = getUser(r, reservation.UserID)

	// 	s.Reservations = append(s.Reservations, reservation)
	// 	reserved++
	// }
	// s.Reserved = reserved

	// return nil

	currentUser := getCurrentUser(r)

	rows, err := db.QueryxContext(r.Context(), `
		SELECT
			r.*,
			u.id,
			u.email,
			u.nickname,
			u.staff,
			u.created_at
		FROM reservations r
		JOIN users u ON r.user_id = u.id
		WHERE r.schedule_id = ?
	`, s.ID)
	if err != nil {
		return err
	}

	var reservations []*Reservation

	if currentUser != nil && currentUser.Staff {
		for rows.Next() {
			reservation := Reservation{}
			user := User{}

			if err := rows.Scan(&reservation.ID, &reservation.ScheduleID, &reservation.UserID, &reservation.CreatedAt, &user.ID, &user.Email, &user.Nickname, &user.Staff, &user.CreatedAt); err != nil {
				return err
			}

			reservation.User = &user

			reservations = append(reservations, &reservation)
		}
	} else {
		for rows.Next() {
			reservation := Reservation{}
			user := User{}
			tmp := ""

			if err := rows.Scan(&reservation.ID, &reservation.ScheduleID, &reservation.UserID, &reservation.CreatedAt, &user.ID, &tmp, &user.Nickname, &user.Staff, &user.CreatedAt); err != nil {
				return err
			}

			reservation.User = &user

			reservations = append(reservations, &reservation)
		}
	}

	s.Reserved = len(reservations)
	s.Reservations = reservations

	return nil
}

// func getReservationsCount(r *http.Request, s *Schedule) error {
// 	rows, err := db.QueryxContext(r.Context(), "SELECT * FROM `reservations` WHERE `schedule_id` = ?", s.ID)
// 	if err != nil {
// 		return err
// 	}

// 	defer rows.Close()

// 	reserved := 0
// 	for rows.Next() {
// 		reserved++
// 	}
// 	s.Reserved = reserved

// 	return nil
// }

// func getUser(r *http.Request, id string) *User {
// 	user := &User{}
// 	if err := db.QueryRowxContext(r.Context(), "SELECT * FROM `users` WHERE `id` = ? LIMIT 1", id).StructScan(user); err != nil {
// 		return nil
// 	}
// 	if getCurrentUser(r) != nil && !getCurrentUser(r).Staff {
// 		user.Email = ""
// 	}
// 	return user
// }

func parseForm(r *http.Request) error {
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
		return r.ParseForm()
	} else {
		return r.ParseMultipartForm(32 << 20)
	}
}

func serveMux() http.Handler {
	router := mux.NewRouter()

	router.HandleFunc("/initialize", initializeHandler).Methods("POST")
	router.HandleFunc("/api/session", sessionHandler).Methods("GET")
	router.HandleFunc("/api/signup", signupHandler).Methods("POST")
	router.HandleFunc("/api/login", loginHandler).Methods("POST")
	router.HandleFunc("/api/schedules", createScheduleHandler).Methods("POST")
	router.HandleFunc("/api/reservations", createReservationHandler).Methods("POST")
	router.HandleFunc("/api/schedules", schedulesHandler).Methods("GET")
	router.HandleFunc("/api/schedules/{id}", scheduleHandler).Methods("GET")

	dir, err := filepath.Abs(filepath.Join(filepath.Dir(os.Args[0]), "..", "public"))
	if err != nil {
		log.Fatal(err)
	}
	publicDir = dir
	fs = http.FileServer(http.Dir(publicDir))

	router.PathPrefix("/").HandlerFunc(htmlHandler)

	return logger(router)
}

func logger(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		before := time.Now()
		handler.ServeHTTP(w, r)
		after := time.Now()
		duration := after.Sub(before)
		log.Printf("%s % 4s %s (%s)", r.RemoteAddr, r.Method, r.URL.Path, duration)
	})
}

func sendJSON(w http.ResponseWriter, data interface{}, statusCode int) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)

	enc := json.NewEncoder(w)
	return enc.Encode(data)
}

func sendErrorJSON(w http.ResponseWriter, err error, statusCode int) error {
	log.Printf("ERROR: %+v", err)

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)

	enc := json.NewEncoder(w)
	return enc.Encode(map[string]string{"error": err.Error()})
}

type initializeResponse struct {
	Language string `json:"language"`
}

func initializeHandler(w http.ResponseWriter, r *http.Request) {
	err := transaction(r.Context(), &sql.TxOptions{}, func(ctx context.Context, tx *sqlx.Tx) error {
		if _, err := tx.ExecContext(ctx, "TRUNCATE `reservations`"); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "TRUNCATE `schedules`"); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "TRUNCATE `users`"); err != nil {
			return err
		}

		id := generateID()
		if _, err := tx.ExecContext(
			ctx,
			"INSERT INTO `users` (`id`, `email`, `nickname`, `staff`, `created_at`) VALUES (?, ?, ?, true, NOW(6))",
			id,
			"isucon2021_prior@isucon.net",
			"isucon",
		); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		sendErrorJSON(w, err, 500)
	} else {
		sendJSON(w, initializeResponse{Language: "golang"}, 200)
	}
}

func sessionHandler(w http.ResponseWriter, r *http.Request) {
	sendJSON(w, getCurrentUser(r), 200)
}

func signupHandler(w http.ResponseWriter, r *http.Request) {
	if err := parseForm(r); err != nil {
		sendErrorJSON(w, err, 500)
		return
	}

	user := &User{}

	err := transaction(r.Context(), &sql.TxOptions{}, func(ctx context.Context, tx *sqlx.Tx) error {
		email := r.FormValue("email")
		nickname := r.FormValue("nickname")
		id := generateID()

		createdAt := time.Now()
		if _, err := tx.ExecContext(
			ctx,
			"INSERT INTO `users` (`id`, `email`, `nickname`, `created_at`) VALUES (?, ?, ?, ?)",
			id, email, nickname, createdAt,
		); err != nil {
			return err
		}
		user.ID = id
		user.Email = email
		user.Nickname = nickname
		user.CreatedAt = createdAt

		return nil
	})

	if err != nil {
		sendErrorJSON(w, err, 500)
	} else {
		sendJSON(w, user, 200)
	}
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	if err := parseForm(r); err != nil {
		sendErrorJSON(w, err, 500)
		return
	}

	email := r.PostFormValue("email")
	user := &User{}

	if err := db.QueryRowxContext(
		r.Context(),
		"SELECT * FROM `users` WHERE `email` = ? LIMIT 1",
		email,
	).StructScan(user); err != nil {
		sendErrorJSON(w, err, 403)
		return
	}
	cookie := &http.Cookie{
		Name:     "user_id",
		Value:    user.ID,
		Path:     "/",
		MaxAge:   86400,
		HttpOnly: true,
	}
	http.SetCookie(w, cookie)

	sendJSON(w, user, 200)
}

func createScheduleHandler(w http.ResponseWriter, r *http.Request) {
	if err := parseForm(r); err != nil {
		sendErrorJSON(w, err, 500)
		return
	}

	if !requiredStaffLogin(w, r) {
		return
	}

	schedule := &Schedule{}
	err := transaction(r.Context(), &sql.TxOptions{}, func(ctx context.Context, tx *sqlx.Tx) error {
		id := generateID()
		title := r.PostFormValue("title")
		capacity, _ := strconv.Atoi(r.PostFormValue("capacity"))

		createdAt := time.Now()
		if _, err := tx.ExecContext(
			ctx,
			"INSERT INTO `schedules` (`id`, `title`, `capacity`, `created_at`) VALUES (?, ?, ?, ?)",
			id, title, capacity, createdAt,
		); err != nil {
			return err
		}

		schedule.ID = id
		schedule.Title = title
		schedule.Capacity = capacity
		schedule.CreatedAt = createdAt

		return nil
	})

	if err != nil {
		sendErrorJSON(w, err, 500)
	} else {
		sendJSON(w, schedule, 200)
	}
}

func createReservationHandler(w http.ResponseWriter, r *http.Request) {
	if err := parseForm(r); err != nil {
		sendErrorJSON(w, err, 500)
		return
	}
	currentUser := getCurrentUser(r)
	if !requiredLogin(w, r, currentUser) {
		return
	}

	reservation := &Reservation{}
	err := transaction(r.Context(), &sql.TxOptions{}, func(ctx context.Context, tx *sqlx.Tx) error {
		id := generateID()
		scheduleID := r.PostFormValue("schedule_id")
		userID := currentUser.ID

		// found := 0
		schedule := &Schedule{}
		if err := tx.QueryRowxContext(ctx, "SELECT * FROM `schedules` WHERE `id` = ? LIMIT 1 FOR UPDATE", scheduleID).StructScan(schedule); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return sendErrorJSON(w, fmt.Errorf("schedule not found"), 403)
			}
			return sendErrorJSON(w, err, 500)
		}

		// found = 0
		// tx.QueryRowContext(ctx, "SELECT 1 FROM `users` WHERE `id` = ? LIMIT 1", userID).Scan(&found)
		// if found != 1 {
		// 	return sendErrorJSON(w, fmt.Errorf("user not found"), 403)
		// }

		rows, err := tx.QueryxContext(ctx, "SELECT `user_id` FROM `reservations` WHERE `schedule_id` = ?", scheduleID)
		if err != nil {
			return sendErrorJSON(w, err, 500)
		}

		reserved := 0
		for rows.Next() {
			tmp := ""
			if err := rows.Scan(&tmp); err != nil {
				return sendErrorJSON(w, err, 500)
			}
			if tmp == userID {
				return sendErrorJSON(w, fmt.Errorf("already taken"), 403)
			}
			reserved++
		}

		// capacity := 0
		// if err := tx.QueryRowContext(ctx, "SELECT `capacity` FROM `schedules` WHERE `id` = ? LIMIT 1", scheduleID).Scan(&capacity); err != nil {
		// 	return sendErrorJSON(w, err, 500)
		// }

		capacity := schedule.Capacity

		// reserved := 0
		// err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM `reservations` WHERE `schedule_id` = ?", scheduleID).Scan(&reserved)
		// if err != nil && err != sql.ErrNoRows {
		// 	return sendErrorJSON(w, err, 500)
		// }

		if reserved >= capacity {
			return sendErrorJSON(w, fmt.Errorf("capacity is already full"), 403)
		}

		createdAt := time.Now()
		if _, err := tx.ExecContext(
			ctx,
			"INSERT INTO `reservations` (`id`, `schedule_id`, `user_id`, `created_at`) VALUES (?, ?, ?, ?)",
			id, scheduleID, userID, createdAt,
		); err != nil {
			return err
		}
		reservation.ID = id
		reservation.ScheduleID = scheduleID
		reservation.UserID = userID
		reservation.CreatedAt = createdAt

		return sendJSON(w, reservation, 200)
	})
	if err != nil {
		sendErrorJSON(w, err, 500)
	}
}

func schedulesHandler(w http.ResponseWriter, r *http.Request) {
	schedules := []*Schedule{}
	rows, err := db.QueryxContext(r.Context(), `
		SELECT
			s.id,
			s.title,
			s.capacity,
			r.reserved,
			s.created_at
		FROM schedules s
		LEFT JOIN (SELECT schedule_id, COUNT(*) reserved FROM reservations GROUP BY schedule_id) r
		ON s.id = r.schedule_id
		ORDER BY s.id DESC
	`)
	if err != nil {
		sendErrorJSON(w, err, 500)
		return
	}

	currentUser := getCurrentUser(r)
	if currentUser != nil && currentUser.Staff {
		for rows.Next() {
			schedule := &Schedule{}
			var reserved sql.NullInt64
			if err := rows.Scan(&schedule.ID, &schedule.Title, &schedule.Capacity, &reserved, &schedule.CreatedAt); err != nil {
				sendErrorJSON(w, err, 500)
				return
			}

			if reserved.Valid {
				schedule.Reserved = int(reserved.Int64)
			}

			schedules = append(schedules, schedule)
		}
	} else {
		for rows.Next() {
			schedule := &Schedule{}
			var reserved sql.NullInt64
			if err := rows.Scan(&schedule.ID, &schedule.Title, &schedule.Capacity, &reserved, &schedule.CreatedAt); err != nil {
				sendErrorJSON(w, err, 500)
				return
			}

			if reserved.Valid {
				schedule.Reserved = int(reserved.Int64)
			}

			if schedule.Reserved >= schedule.Capacity {
				continue
			}

			schedules = append(schedules, schedule)
		}
	}

	sendJSON(w, schedules, 200)
}

func scheduleHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	schedule := &Schedule{}
	if err := db.QueryRowxContext(r.Context(), "SELECT * FROM `schedules` WHERE `id` = ? LIMIT 1", id).StructScan(schedule); err != nil {

		sendErrorJSON(w, err, 500)
		return
	}

	if err := getReservations(r, schedule); err != nil {
		sendErrorJSON(w, err, 500)
		return
	}

	sendJSON(w, schedule, 200)
}

func htmlHandler(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	realpath := filepath.Join(publicDir, path)

	if stat, err := os.Stat(realpath); !os.IsNotExist(err) && !stat.IsDir() {
		fs.ServeHTTP(w, r)
		return
	} else {
		realpath = filepath.Join(publicDir, "index.html")
	}

	file, err := os.Open(realpath)
	if err != nil {
		sendErrorJSON(w, err, 500)
		return
	}
	defer file.Close()

	w.Header().Set("Content-Type", "text/html; chartset=utf-8")
	w.WriteHeader(200)
	io.Copy(w, file)
}
