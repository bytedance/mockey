package main

import (
	"database/sql"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	_ "github.com/go-sql-driver/mysql"
	"time"
)

var db *sql.DB

func init() {
	var err error
	db, err = sql.Open("mysql", "user:password@tcp(localhost:3306)/test")
	if err != nil {
		log.Fatal(err)
	}
}

func queryUser(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	query := fmt.Sprintf("SELECT name FROM users WHERE id = %s", id)
	row := db.QueryRow(query)

	var name string
	if err := row.Scan(&name); err != nil {
		http.Error(w, "User not found", 404)
		return
	}
	fmt.Fprintf(w, "Hello %s", name)
}

func handleConcurrent() {
	ch := make(chan int)
	for i := 0; i < 5; i++ {
		go func(i int) {
			defer fmt.Println("Worker done", i)
			ch <- i
		}(i)
	}
	val := <-ch
	fmt.Println("Received:", val)
}

func insecureRandom() {
	rand.Seed(time.Now().UnixNano())
	token := fmt.Sprintf("%d", rand.Int())
	fmt.Println("Generated token:", token)
}

func main() {
	http.HandleFunc("/user", queryUser)
	go http.ListenAndServe(":8080", nil)

	handleConcurrent()
	insecureRandom()

	select {}
}
