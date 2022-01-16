package main

import (
	"encoding/json"
	"errors"
	"io/fs"
	"io/ioutil"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/render"
)

const store = `users.json`

type (
	User struct {
		CreatedAt   time.Time `json:"created_at"`
		DisplayName string    `json:"display_name"`
		Email       string    `json:"email"`
	}
	UserList  map[string]User
	UserStore struct {
		sync.Mutex
		Increment int      `json:"increment"`
		List      UserList `json:"list"`
	}
)

var (
	ErrUserNotFound           = errors.New("user not found")
	ErrUserDisplayNameIsEmpty = errors.New(`display name must not be empty`)
)

func (u User) Validate() error {
	if u.DisplayName == "" {
		return ErrUserDisplayNameIsEmpty
	}

	return nil
}

func (s *UserStore) saveToJSONFile() error {
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}

	if err = ioutil.WriteFile(store, b, fs.ModePerm); err != nil {
		return err
	}

	return nil
}

func (s *UserStore) GetUser(id string) (*User, error) {
	s.Lock()
	defer s.Unlock()

	user, ok := s.List[id]
	if !ok {
		return nil, ErrUserNotFound
	}

	return &user, nil
}

func (s *UserStore) CreateUser(name string, email string) (string, error) {
	s.Lock()
	defer s.Unlock()

	s.Increment++
	user := User{
		CreatedAt:   time.Now(),
		DisplayName: name,
		Email:       email,
	}

	if err := user.Validate(); err != nil {
		return "", ErrUserDisplayNameIsEmpty
	}

	id := strconv.Itoa(s.Increment)
	s.List[id] = user

	if err := s.saveToJSONFile(); err != nil {
		return "", err
	}

	return id, nil
}

func (s *UserStore) UpdateUser(id, name string) error {
	s.Lock()
	defer s.Unlock()

	user, ok := s.List[id]
	if !ok {
		return ErrUserNotFound
	}

	user.DisplayName = name
	if err := user.Validate(); err != nil {
		return ErrUserDisplayNameIsEmpty
	}

	s.List[id] = user

	if err := s.saveToJSONFile(); err != nil {
		return err
	}

	return nil
}

func (s *UserStore) DeleteUser(id string) error {
	s.Lock()
	defer s.Unlock()

	_, ok := s.List[id]
	if !ok {
		return ErrUserNotFound
	}

	delete(s.List, id)

	if err := s.saveToJSONFile(); err != nil {
		return err
	}

	return nil
}

func main() {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(time.Now().String()))
	})

	r.Route("/api", func(r chi.Router) {
		r.Route("/v1", func(r chi.Router) {
			r.Route("/users", func(r chi.Router) {
				r.Get("/", searchUsers)
				r.Post("/", createUser)

				r.Route("/{id}", func(r chi.Router) {
					r.Get("/", getUser)
					r.Patch("/", updateUser)
					r.Delete("/", deleteUser)
				})
			})
		})
	})

	http.ListenAndServe(":3333", r)
}

func searchUsers(w http.ResponseWriter, r *http.Request) {
	f, _ := ioutil.ReadFile(store)
	s := UserStore{}
	_ = json.Unmarshal(f, &s)

	render.JSON(w, r, s.List)
}

type CreateUserRequest struct {
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
}

func (c *CreateUserRequest) Bind(r *http.Request) error { return nil }

func createUser(w http.ResponseWriter, r *http.Request) {
	f, err := ioutil.ReadFile(store)
	if err != nil {
		panic(err)
	}

	s := UserStore{}
	err = json.Unmarshal(f, &s)
	if err != nil {
		panic(err)
	}

	defer r.Body.Close()

	request := CreateUserRequest{}

	if err := render.Bind(r, &request); err != nil {
		err = render.Render(w, r, ErrInvalidRequest(
			err,
			http.StatusBadRequest,
			http.StatusText(http.StatusBadRequest)),
		)
		if err != nil {
			panic(err)
		}
		return
	}

	id, err := s.CreateUser(request.DisplayName, request.Email)
	if err != nil {
		switch err {
		case ErrUserDisplayNameIsEmpty:
			err = render.Render(w, r, ErrInvalidRequest(
				ErrUserDisplayNameIsEmpty,
				http.StatusBadRequest,
				http.StatusText(http.StatusBadRequest)),
			)
			if err != nil {
				panic(err)
			}
		default:
			if err != nil {
				panic(err)
			}
		}
		return
	}

	render.Status(r, http.StatusCreated)
	render.JSON(w, r, map[string]interface{}{
		"user_id": id,
	})
}

func getUser(w http.ResponseWriter, r *http.Request) {
	f, err := ioutil.ReadFile(store)
	if err != nil {
		panic(err)
	}

	s := UserStore{}
	err = json.Unmarshal(f, &s)
	if err != nil {
		panic(err)
	}

	id := chi.URLParam(r, "id")

	user, err := s.GetUser(id)
	if err != nil {
		err = render.Render(w, r, ErrInvalidRequest(
			ErrUserNotFound,
			http.StatusNotFound,
			http.StatusText(http.StatusNotFound)),
		)
		if err != nil {
			panic(err)
		}
		return
	}
	render.JSON(w, r, user)
}

type UpdateUserRequest struct {
	DisplayName string `json:"display_name"`
}

func (c *UpdateUserRequest) Bind(r *http.Request) error { return nil }

func updateUser(w http.ResponseWriter, r *http.Request) {
	f, err := ioutil.ReadFile(store)
	if err != nil {
		panic(err)
	}

	s := UserStore{}
	err = json.Unmarshal(f, &s)
	if err != nil {
		panic(err)
	}

	request := UpdateUserRequest{}

	if err := render.Bind(r, &request); err != nil {
		err = render.Render(w, r, ErrInvalidRequest(
			err,
			http.StatusBadRequest,
			http.StatusText(http.StatusBadRequest)),
		)
		if err != nil {
			panic(err)
		}
		return
	}

	id := chi.URLParam(r, "id")

	if err := s.UpdateUser(id, request.DisplayName); err != nil {
		switch err {
		case ErrUserDisplayNameIsEmpty:
			err = render.Render(w, r, ErrInvalidRequest(
				ErrUserDisplayNameIsEmpty,
				http.StatusBadRequest,
				http.StatusText(http.StatusBadRequest)),
			)
			if err != nil {
				panic(err)
			}
		case ErrUserNotFound:
			err = render.Render(w, r, ErrInvalidRequest(
				ErrUserNotFound,
				http.StatusNotFound,
				http.StatusText(http.StatusNotFound)),
			)
			if err != nil {
				panic(err)
			}
		default:
			if err != nil {
				panic(err)
			}
		}
		return
	}

	render.Status(r, http.StatusNoContent)
}

func deleteUser(w http.ResponseWriter, r *http.Request) {
	f, err := ioutil.ReadFile(store)
	if err != nil {
		panic(err)
	}

	s := UserStore{}
	err = json.Unmarshal(f, &s)
	if err != nil {
		panic(err)
	}

	id := chi.URLParam(r, "id")

	if err := s.DeleteUser(id); err != nil {
		switch err {
		case ErrUserNotFound:
			err = render.Render(w, r, ErrInvalidRequest(
				ErrUserNotFound,
				http.StatusNotFound,
				http.StatusText(http.StatusNotFound)),
			)
			if err != nil {
				panic(err)
			}
		default:
			if err != nil {
				panic(err)
			}
		}
		return
	}

	render.Status(r, http.StatusNoContent)
}

type ErrResponse struct {
	Err            error `json:"-"`
	HTTPStatusCode int   `json:"-"`

	StatusText string `json:"status"`
	AppCode    int64  `json:"code,omitempty"`
	ErrorText  string `json:"error,omitempty"`
}

func (e *ErrResponse) Render(w http.ResponseWriter, r *http.Request) error {
	render.Status(r, e.HTTPStatusCode)
	return nil
}

func ErrInvalidRequest(err error, status int, statusText string) render.Renderer {
	return &ErrResponse{
		Err:            err,
		HTTPStatusCode: status,
		StatusText:     statusText,
		ErrorText:      err.Error(),
	}
}
