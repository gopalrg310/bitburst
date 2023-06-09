package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/gopalrg310/bitburst/models"
	"github.com/gopalrg310/bitburst/utils"
	"github.com/gorilla/mux"
	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/sirupsen/logrus"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type UserService struct {
	Db *pgxpool.Pool
}

func NewUserService() *UserService {
	return new(UserService)
}
func (s *UserService) AddTransactionHandler(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	serviceName := r.URL.Path
	tokens := strings.Split(serviceName, "/")
	submodule := strings.ToUpper(tokens[1])
	defer utils.InitialRecover(w, submodule, serviceName)
	requestLogger := utils.Log.WithFields(logrus.Fields{"module": submodule, "UrlPath": serviceName})
	vars := mux.Vars(r)
	userID := vars["uid"]
	var request struct {
		Amount float64 `json:"amount"`
	}

	err := json.NewDecoder(r.Body).Decode(&request)
	if err != nil {
		msg := err.Error()
		utils.ResponseHandler(w, requestLogger, serviceName, "json", http.StatusInternalServerError, utils.FormJsonOutput(msg), 404, "", nil, "", err.Error())
		return
	}
	if request.Amount <= 0 {
		msg := "Amount must be positive"
		utils.ResponseHandler(w, requestLogger, serviceName, "json", http.StatusInternalServerError, utils.FormJsonOutput(msg), 404, "", nil, "", nil)
		return
	}
	// Create a new transaction
	transaction := models.UserTransaction{
		UserID:        userID,
		Amount:        request.Amount,
		TransactionID: uuid.New().String(),
		Timestamp:     time.Now(),
	}

	// Insert the transaction into the database
	requestLogger.Info(transaction)
	err = utils.AddTransaction(s.Db, transaction)
	if err != nil {
		if pqErr, ok := err.(*pgconn.PgError); ok {
			if pqErr.Code == "23505" {
				msg := "Handle duplicate transaction error"
				utils.ResponseHandler(w, requestLogger, serviceName, "json", http.StatusInternalServerError, utils.FormJsonOutput(msg), 404, "", nil, "", err.Error())
				return
			}
		}
		// Handle other errors
		msg := "Error inserting transaction"
		utils.ResponseHandler(w, requestLogger, serviceName, "json", http.StatusInternalServerError, utils.FormJsonOutput(msg), 404, "", nil, "", err.Error())
		return
	}

	msg := fmt.Sprintf("Transaction of $%2f added for user %s", request.Amount, userID)
	utils.ResponseHandler(w, requestLogger, serviceName, "json", http.StatusOK, utils.FormJsonOutput(msg), 200, utils.FormJsonOutput(msg), nil, "", nil)
	elapsedTime := time.Since(startTime).Seconds()
	requestLogger.Info("Response time : ", elapsedTime)
}

func (s *UserService) BalanceHandler(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	serviceName := r.URL.Path
	tokens := strings.Split(serviceName, "/")
	submodule := strings.ToUpper(tokens[1])
	defer utils.InitialRecover(w, submodule, serviceName)
	requestLogger := utils.Log.WithFields(logrus.Fields{"module": submodule, "UrlPath": serviceName})
	vars := mux.Vars(r)
	userID := vars["uid"]

	row, err := s.Db.Query(context.Background(), "select coalesce(sum(amount), 0) from transactions where user_id=$1;", userID)
	if err != nil {
		msg := "Error initiate query"
		utils.ResponseHandler(w, requestLogger, serviceName, "json", http.StatusInternalServerError, utils.FormJsonOutput(msg), 404, "", nil, "", err.Error())
		return
	}
	var balance float64
	for row.Next() {
		err = row.Scan(&balance)
		if err != nil {
			msg := "Error querying balanceHandler"
			utils.ResponseHandler(w, requestLogger, serviceName, "json", http.StatusInternalServerError, utils.FormJsonOutput(msg), 404, "", nil, "", err.Error())
			return
		}
	}

	msg := fmt.Sprintf("User %s has a balance of $%2f", userID, balance)
	utils.ResponseHandler(w, requestLogger, serviceName, "json", http.StatusOK, utils.FormJsonOutput(msg), 200, utils.FormJsonOutput(msg), nil, "", nil)
	elapsedTime := time.Since(startTime).Seconds()
	requestLogger.Info("Response time : ", elapsedTime)
}

func (s *UserService) HistoryHandler(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	serviceName := r.URL.Path
	tokens := strings.Split(serviceName, "/")
	submodule := strings.ToUpper(tokens[1])
	defer utils.InitialRecover(w, submodule, serviceName)
	requestLogger := utils.Log.WithFields(logrus.Fields{"module": submodule, "UrlPath": serviceName})

	vars := mux.Vars(r)
	userID := vars["uid"]

	pageStr := r.URL.Query().Get("page")
	page, err := strconv.Atoi(pageStr)
	if err != nil || page < 1 {
		page = 1
	}
	requestLogger.Info(page)
	offset := (page - 1) * 10

	rows, err := s.Db.Query(context.Background(), "SELECT user_id, amount, created_at FROM transactions WHERE user_id=$1 ORDER BY created_at DESC LIMIT 10 OFFSET $2", userID, offset)
	if err != nil {
		msg := "Error getting transaction history"
		utils.ResponseHandler(w, requestLogger, serviceName, "json", http.StatusInternalServerError, utils.FormJsonOutput(msg), 404, "", nil, "", err.Error())
		return
	}
	defer rows.Close()

	var transactions []models.UserTransaction

	for rows.Next() {
		var transaction models.UserTransaction
		err := rows.Scan(&transaction.UserID, &transaction.Amount, &transaction.Timestamp)
		if err != nil {
			msg := "Error in constructing response"
			utils.ResponseHandler(w, requestLogger, serviceName, "json", http.StatusInternalServerError, utils.FormJsonOutput(msg), 404, "", nil, "", err.Error())
			return
		}
		transactions = append(transactions, transaction)
	}
	if len(transactions) == 0 {
		utils.ResponseHandler(w, requestLogger, serviceName, "json", http.StatusOK, "{}", 200, "", nil, "", nil)
		return
	}
	resp, err := json.Marshal(transactions)
	if err != nil {
		msg := "Error in constructing response"
		utils.ResponseHandler(w, requestLogger, serviceName, "json", http.StatusNotFound, utils.FormJsonOutput(msg), 404, "", nil, "", err.Error())
		return
	}
	msg := fmt.Sprintf("User %s has a history of transaction $%d", userID, transactions)
	utils.ResponseHandler(w, requestLogger, serviceName, "json", http.StatusOK, string(resp), 200, utils.FormJsonOutput(msg), nil, "", nil)
	elapsedTime := time.Since(startTime).Seconds()
	requestLogger.Info("Response time : ", elapsedTime)
}
