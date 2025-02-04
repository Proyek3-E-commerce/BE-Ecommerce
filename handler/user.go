package handler

import (
	"be_ecommerce/config"
	"be_ecommerce/model"
	"be_ecommerce/utils"
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"golang.org/x/crypto/bcrypt"
)

// Helper function to get user collection
func getUserCollection() *mongo.Collection {
	return config.MongoClient.Database("ecommerce").Collection("users")
}

// CRUD for Customers
func GetCustomers(c *fiber.Ctx) error {
	collection := getUserCollection()

	// Query untuk mendapatkan semua pelanggan dengan role "customer"
	filter := bson.M{"roles": "customer"}
	cursor, err := collection.Find(context.Background(), filter)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Error fetching customers",
		})
	}
	defer cursor.Close(context.Background())

	// Parsing hasil query ke dalam slice
	var customers []bson.M
	if err := cursor.All(context.Background(), &customers); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Error parsing customers",
		})
	}

	// Transform data untuk menyederhanakan output
	transformedCustomers := make([]map[string]interface{}, 0)
	for _, customer := range customers {
		transformed := map[string]interface{}{
			"id":        customer["_id"],
			"username":  customer["username"],
			"email":     customer["email"],
			"roles":     customer["roles"],
		}
		transformedCustomers = append(transformedCustomers, transformed)
	}

	// Kembalikan data ke frontend
	return c.JSON(transformedCustomers)
}

func GetUserProfile(c *fiber.Ctx) error {
	// Ambil token dari header Authorization
	token := c.Get("Authorization")
	if token == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Authorization token is required",
		})
	}

	// Hapus prefix "Bearer " dari token
	token = strings.TrimPrefix(token, "Bearer ")

	// Validasi token dan ambil klaim
	claims, err := utils.ValidateJWT(token)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Invalid or expired token",
		})
	}

	// Ambil user_id dari klaim
	userID, ok := claims["user_id"].(string)
	if !ok || userID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Invalid token payload",
		})
	}

	// Konversi user_id ke ObjectID
	objectID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid user ID format",
		})
	}

	// Ambil data pengguna dari database
	collection := config.MongoClient.Database("ecommerce").Collection("users")
	var user model.User
	err = collection.FindOne(context.Background(), bson.M{"_id": objectID}).Decode(&user)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"message": "User not found",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to fetch user data",
		})
	}

	// Siapkan respons dengan data user (hapus password)
	user.Password = ""
	response := fiber.Map{
		"id":       user.ID.Hex(),
		"username": user.Username,
		"email":    user.Email,
		"roles":    user.Roles,
	}

	// Tambahkan `store_status` jika ada
	if user.StoreStatus != nil {
		response["store_status"] = *user.StoreStatus
	}

	// Tambahkan `store_info` jika ada
	if user.StoreInfo != nil {
		response["store_info"] = user.StoreInfo
	}

	// Kembalikan respons
	return c.JSON(fiber.Map{
		"message": "User profile fetched successfully",
		"data":    response,
	})
}

func EditProfile(c *fiber.Ctx) error {
	// Ambil token dari header Authorization
	authHeader := c.Get("Authorization")
	if authHeader == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Authorization token is required",
		})
	}

	// Periksa format Bearer token
	token := strings.TrimPrefix(authHeader, "Bearer ")
	if token == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Invalid token format",
		})
	}

	// Validasi token
	claims, err := utils.ValidateJWT(token)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Invalid or expired token",
		})
	}

	// Ambil user ID dari klaim token
	userID, ok := claims["user_id"].(string)
	if !ok || userID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Invalid token payload",
		})
	}

	// Konversi user ID ke ObjectID
	objectID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid user ID",
		})
	}

	// Parsing data dari request body
	var updatedData struct {
		Username string `json:"username"`
	}
	if err := c.BodyParser(&updatedData); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
		})
	}

	// Validasi username
	if updatedData.Username == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Username cannot be empty",
		})
	}

	// Update username di database
	collection := config.MongoClient.Database("ecommerce").Collection("users")
	_, err = collection.UpdateOne(
		context.Background(),
		bson.M{"_id": objectID},
		bson.M{"$set": bson.M{
			"username": updatedData.Username,
		}},
	)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to update profile",
		})
	}

	return c.JSON(fiber.Map{
		"message": "Profile updated successfully",
	})
}

func VerifyOTP(c *fiber.Ctx) error {
	// Parsing data dari request body
	var body struct {
		Email      string `json:"email"`
		ResetToken string `json:"reset_token"`
	}
	if err := c.BodyParser(&body); err != nil {
		log.Println("Error parsing request body:", err)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
		})
	}

	log.Println("Verifying OTP for email:", body.Email, "with token:", body.ResetToken)

	// Cek token reset
	collection := config.MongoClient.Database("ecommerce").Collection("users")
	var user model.User
	err := collection.FindOne(
		context.Background(),
		bson.M{"email": body.Email, "reset_token": body.ResetToken},
	).Decode(&user)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			log.Println("Invalid or expired token for email:", body.Email)
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"message": "Invalid OTP",
			})
		}
		log.Println("Database error:", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Server error",
		})
	}

	// Periksa apakah token telah kedaluwarsa
	if time.Now().After(user.ResetTokenExpiry) {
		log.Println("OTP expired for email:", body.Email)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "OTP expired",
		})
	}

	log.Println("OTP verified successfully for email:", body.Email)
	return c.JSON(fiber.Map{
		"message": "OTP verified successfully",
	})
}

func SendPasswordResetEmail(c *fiber.Ctx) error {
	// Parsing email dari request body
	var body struct {
		Email string `json:"email"`
	}
	if err := c.BodyParser(&body); err != nil {
		log.Println("Error parsing request body:", err)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
		})
	}

	// Cek apakah email terdaftar di database
	collection := config.MongoClient.Database("ecommerce").Collection("users")
	var user model.User
	err := collection.FindOne(context.Background(), bson.M{"email": body.Email}).Decode(&user)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			log.Println("Email not found in database:", body.Email)
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"message": "Email not found",
			})
		}
		log.Println("Database error while fetching user:", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to fetch user data",
		})
	}

	// Generate OTP dan expiry time
	resetToken := utils.GenerateRandomToken(6) // Contoh fungsi utilitas untuk membuat token OTP
	expiry := time.Now().Add(10 * time.Minute) // OTP berlaku selama 10 menit

	// Simpan OTP dan expiry ke database
	_, err = collection.UpdateOne(
		context.Background(),
		bson.M{"email": body.Email},
		bson.M{"$set": bson.M{
			"reset_token":       resetToken,
			"reset_token_expiry": expiry,
		}},
	)
	if err != nil {
		log.Println("Error saving OTP to database:", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to save OTP",
		})
	}

	// Kirim OTP ke email pengguna
	err = utils.SendEmail(body.Email, "Password Reset", fmt.Sprintf("Your OTP: %s", resetToken))
	if err != nil {
		log.Println("Error sending email:", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to send email",
		})
	}

	log.Println("Password reset email sent successfully to:", body.Email)
	return c.JSON(fiber.Map{
		"message": "Password reset email sent successfully",
	})
}

func ResetPassword(c *fiber.Ctx) error {
	// Parsing data dari request body
	var body struct {
		Email       string `json:"email"`
		ResetToken  string `json:"reset_token"`
		NewPassword string `json:"new_password"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
		})
	}

	// Validasi reset token
	collection := config.MongoClient.Database("ecommerce").Collection("users")
	var user model.User
	err := collection.FindOne(context.Background(), bson.M{
		"email":       body.Email,
		"reset_token": body.ResetToken,
	}).Decode(&user)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"message": "Invalid or expired reset token",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to verify reset token",
		})
	}

	// Cek apakah token sudah kedaluwarsa
	if time.Now().After(user.ResetTokenExpiry) {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Reset token expired",
		})
	}

	// Hash password baru
	hashedPassword, err := utils.HashPassword(body.NewPassword)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to hash password",
		})
	}

	// Update password dan hapus reset token
	_, err = collection.UpdateOne(
		context.Background(),
		bson.M{"email": body.Email},
		bson.M{
			"$set": bson.M{"password": hashedPassword},
			"$unset": bson.M{"reset_token": "", "reset_token_expiry": ""},
		},
	)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to update password",
		})
	}

	return c.JSON(fiber.Map{
		"message": "Password reset successfully",
	})
}


// Fungsi untuk hashing password
func hashPassword(password string) (string, error) {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hashedPassword), nil
}

func CreateCustomer(c *fiber.Ctx) error {
	var user model.User
	if err := c.BodyParser(&user); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
		})
	}

	// Hashing password
	hashedPassword, err := hashPassword(user.Password)
	if err != nil {
		log.Println("Error hashing password:", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Error processing password",
		})
	}
	user.Password = hashedPassword

	// Set roles and ID
	user.Roles = []string{"customer"}
	user.ID = primitive.NewObjectID()

	// Simpan ke database
	collection := getUserCollection()
	_, err = collection.InsertOne(context.Background(), user)
	if err != nil {
		log.Println("Error creating customer:", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Error creating customer",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Customer created successfully",
	})
}
func UpdateCustomer(c *fiber.Ctx) error {
	// Parsing body request
	var body struct {
		UserID  string                 `json:"user_id"` // ID pengguna
		Updates map[string]interface{} `json:"updates"` // Data yang akan diupdate
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
		})
	}

	// Validasi ID pengguna
	if body.UserID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "User ID is required",
		})
	}

	// Konversi UserID ke ObjectID
	userID, err := primitive.ObjectIDFromHex(body.UserID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid User ID format",
		})
	}

	// Validasi: pastikan tidak ada field sensitif yang diperbarui
	disallowedFields := []string{"_id", "password", "roles"}
	for _, field := range disallowedFields {
		if _, exists := body.Updates[field]; exists {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"message": fmt.Sprintf("Field '%s' cannot be updated", field),
			})
		}
	}

	// Pastikan data yang akan diupdate tidak kosong
	if len(body.Updates) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "No updates provided",
		})
	}

	// Update pengguna di database
	collection := getUserCollection()
	filter := bson.M{"_id": userID, "roles": "customer"}
	update := bson.M{"$set": body.Updates}

	result, err := collection.UpdateOne(context.Background(), filter, update)
	if err != nil {
		log.Println("Error updating customer:", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Error updating customer",
		})
	}

	// Periksa apakah ada dokumen yang diperbarui
	if result.MatchedCount == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"message": "Customer not found or no changes made",
		})
	}

	return c.JSON(fiber.Map{
		"message": "Customer updated successfully",
	})
}

func DeleteCustomer(c *fiber.Ctx) error {
	id := c.Params("id")
	userID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid ID",
		})
	}

	collection := getUserCollection()
	_, err = collection.DeleteOne(context.Background(), bson.M{"_id": userID, "roles": "customer"})
	if err != nil {
		log.Println("Error deleting customer:", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Error deleting customer",
		})
	}

	return c.JSON(fiber.Map{
		"message": "Customer deleted successfully",
	})
}

// CRUD for Sellers
func GetSellers(c *fiber.Ctx) error {
	collection := getUserCollection()

	// Query to fetch all users with the role "seller"
	filter := bson.M{
		"$or": []bson.M{
			{"roles": "seller"},
			{"store_status": "rejected"},
		},
	}
	cursor, err := collection.Find(context.Background(), filter)
	if err != nil {
		log.Println("Error fetching sellers:", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Error fetching sellers",
		})
	}
	defer cursor.Close(context.Background())

	// Parse query results into a slice
	var sellers []bson.M
	if err := cursor.All(context.Background(), &sellers); err != nil {
		log.Println("Error decoding sellers:", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Error decoding sellers",
		})
	}

	// Transform the data for simplified output
	transformedSellers := make([]map[string]interface{}, 0)
	for _, seller := range sellers {
		transformed := map[string]interface{}{
			"id":           seller["_id"],
			"username":     seller["username"],
			"email":        seller["email"],
			"roles":        seller["roles"],
			"store_status": seller["store_status"],
			"store_info": map[string]interface{}{
				"store_name":   getStringOrDefault(seller, "store_info", "store_name"),
				"full_address": getStringOrDefault(seller, "store_info", "full_address"),
				"nik":          getStringOrDefault(seller, "store_info", "nik"),
				"photo_path":   getStringOrDefault(seller, "store_info", "photo_path"),
			},
		}

		transformedSellers = append(transformedSellers, transformed)
	}

	// Return data to the frontend
	return c.JSON(fiber.Map{
		"data":    transformedSellers,
		"message": "Sellers fetched successfully",
	})
}

// Helper function to safely get nested strings
func getStringOrDefault(doc bson.M, key string, nestedKey string) string {
	if outer, ok := doc[key].(bson.M); ok {
		if value, ok := outer[nestedKey].(string); ok {
			return value
		}
	}
	return ""
}

// GetUserByID retrieves a user by their ID
func GetUserByID(c *fiber.Ctx) error {
	// Ambil ID dari URL parameter
	userID := c.Params("id")

	// Buat filter untuk query
	var filter bson.M
	if len(userID) == 24 {
		// Jika ID valid sebagai ObjectID
		objectID, err := primitive.ObjectIDFromHex(userID)
		if err == nil {
			filter = bson.M{"_id": objectID}
		}
	} else {
		// Jika ID bukan ObjectID, gunakan sebagai string
		filter = bson.M{"_id": userID}
	}

	// Ambil koleksi user
	userCollection := config.MongoClient.Database("ecommerce").Collection("users")

	// Cari user berdasarkan ID
	var user model.User
	err := userCollection.FindOne(context.Background(), filter).Decode(&user)
	if err != nil {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{
			"status":  "error",
			"message": "User not found",
		})
	}

	// Siapkan respons dengan data user
	response := fiber.Map{
		"id":       user.ID.Hex(),
		"username": user.Username,
		"email":    user.Email,
		"roles":    user.Roles,
		"store_name": func() string {
			if user.StoreInfo != nil {
				return user.StoreInfo.StoreName
			}
			return ""
		}(),
		"full_address": func() string {
			if user.StoreInfo != nil {
				return user.StoreInfo.FullAddress
			}
			return ""
		}(),
		"nik": func() string {
			if user.StoreInfo != nil {
				return user.StoreInfo.NIK
			}
			return ""
		}(),
		"store_status": func() string {
			if user.StoreStatus != nil {
				return *user.StoreStatus
			}
			return "not_available"
		}(),
	}

	// Kembalikan data user
	return c.JSON(fiber.Map{
		"status": "success",
		"data":   response,
	})
}

func GetSellerByID(c *fiber.Ctx) error {
	id := c.Params("id")

	// Konversi ID ke ObjectID MongoDB
	userID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid seller ID",
		})
	}

	// Cari seller berdasarkan ID dan role "seller"
	collection := getUserCollection()
	var seller model.User
	err = collection.FindOne(context.Background(), bson.M{"_id": userID, "roles": "seller"}).Decode(&seller)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"message": "Seller not found",
		})
	}

	// Siapkan respons
	response := fiber.Map{
		"id":       seller.ID.Hex(),
		"username": seller.Username,
		"email":    seller.Email,
		"store_name": func() string {
			if seller.StoreInfo != nil {
				return seller.StoreInfo.StoreName
			}
			return ""
		}(),
		"full_address": func() string {
			if seller.StoreInfo != nil {
				return seller.StoreInfo.FullAddress
			}
			return ""
		}(),
		"nik": func() string {
			if seller.StoreInfo != nil {
				return seller.StoreInfo.NIK
			}
			return ""
		}(),
		"store_status": func() string {
			if seller.StoreStatus != nil {
				return *seller.StoreStatus
			}
			return "unknown"
		}(),
		"suspended": func() bool {
			if seller.StoreStatus != nil && *seller.StoreStatus == "suspended" {
				return true
			}
			return false
		}(),
	}

	return c.JSON(response)
}

func CreateSeller(c *fiber.Ctx) error {
	var user model.User
	if err := c.BodyParser(&user); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
		})
	}

	user.Roles = []string{"seller"}
	user.ID = primitive.NewObjectID()
	collection := getUserCollection()
	_, err := collection.InsertOne(context.Background(), user)
	if err != nil {
		log.Println("Error creating seller:", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Error creating seller",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Seller created successfully",
	})
}

func UpdateSeller(c *fiber.Ctx) error {
	id := c.Params("id")
	userID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid ID",
		})
	}

	var updates bson.M
	if err := c.BodyParser(&updates); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
		})
	}

	collection := getUserCollection()
	_, err = collection.UpdateOne(context.Background(), bson.M{"_id": userID, "roles": "seller"}, bson.M{"$set": updates})
	if err != nil {
		log.Println("Error updating seller:", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Error updating seller",
		})
	}

	return c.JSON(fiber.Map{
		"message": "Seller updated successfully",
	})
}

func UpdateProductForSeller(c *fiber.Ctx) error {
    // Ambil token dari header Authorization
    token := c.Get("Authorization")
    if token == "" {
        return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
            "message": "Unauthorized: Missing token",
        })
    }

    // Validasi token dan ambil klaim
    claims, err := utils.ValidateJWT(token[7:]) // Hapus prefix "Bearer "
    if err != nil {
        return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
            "message": "Unauthorized: Invalid token",
        })
    }

    // Ambil user_id dari klaim
    userID, ok := claims["user_id"].(string)
    if !ok || userID == "" {
        return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
            "message": "Unauthorized: Invalid user ID",
        })
    }

    // Konversi user_id ke ObjectID
    sellerID, err := primitive.ObjectIDFromHex(userID)
    if err != nil {
        return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
            "message": "Invalid User ID format",
        })
    }

    // Ambil product_id dari parameter
    productID := c.Params("id")
    objectID, err := primitive.ObjectIDFromHex(productID)
    if err != nil {
        return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
            "message": "Invalid Product ID format",
        })
    }

    // Cari produk berdasarkan ID dan pastikan milik seller
    productCollection := config.MongoClient.Database("ecommerce").Collection("products")
    var existingProduct model.Product
    err = productCollection.FindOne(context.Background(), bson.M{"_id": objectID, "seller_id": sellerID}).Decode(&existingProduct)
    if err != nil {
        return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
            "message": "Forbidden: You do not have permission to update this product",
        })
    }

    // Parse form data
    form, err := c.MultipartForm()
    if err != nil {
        return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
            "message": "Invalid form data",
            "error":   err.Error(),
        })
    }

    updateData := bson.M{}

    if len(form.Value["name"]) > 0 {
        updateData["name"] = form.Value["name"][0]
    }

    if len(form.Value["price"]) > 0 {
        price, err := strconv.Atoi(form.Value["price"][0])
        if err == nil {
            updateData["price"] = price
        }
    }

    if len(form.Value["stock"]) > 0 {
        stock, err := strconv.Atoi(form.Value["stock"][0])
        if err == nil {
            updateData["stock"] = stock
        }
    }

    if len(form.Value["discount"]) > 0 {
        discount, err := strconv.Atoi(form.Value["discount"][0])
        if err == nil {
            updateData["discount"] = discount
        }
    }

    if len(form.Value["description"]) > 0 {
        updateData["description"] = form.Value["description"][0]
    }

    if len(form.Value["category_id"]) > 0 {
        categoryID, err := primitive.ObjectIDFromHex(form.Value["category_id"][0])
        if err == nil {
            updateData["category_id"] = categoryID
        }
    }

    if len(form.Value["sub_category_id"]) > 0 {
        subCategoryID, err := primitive.ObjectIDFromHex(form.Value["sub_category_id"][0])
        if err == nil {
            updateData["sub_category_id"] = subCategoryID
        }
    }

    // **Update Image jika ada upload file baru**
    fileHeaders := form.File["image"]
    if len(fileHeaders) > 0 {
        file := fileHeaders[0]
        imagePath := fmt.Sprintf("./uploads/%s", file.Filename)

        // Simpan gambar baru
        if err := c.SaveFile(file, imagePath); err != nil {
            return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
                "message": "Failed to save image",
                "error":   err.Error(),
            })
        }

        updateData["image"] = imagePath
    }

    // Update produk di database
    _, err = productCollection.UpdateOne(context.Background(), bson.M{"_id": objectID}, bson.M{"$set": updateData})
    if err != nil {
        return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
            "message": "Failed to update product",
            "error":   err.Error(),
        })
    }

    return c.JSON(fiber.Map{
        "message": "Product updated successfully",
        "status":  "success",
    })
}

func DeleteProductForSeller(c *fiber.Ctx) error {
    // Ambil token dari header Authorization
    token := c.Get("Authorization")
    if token == "" {
        return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
            "message": "Unauthorized: Missing token",
        })
    }

    // Validasi token dan ambil klaim
    claims, err := utils.ValidateJWT(token[7:]) // Hapus prefix "Bearer "
    if err != nil {
        return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
            "message": "Unauthorized: Invalid token",
        })
    }

    // Ambil user_id dari klaim
    userID, ok := claims["user_id"].(string)
    if !ok || userID == "" {
        return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
            "message": "Unauthorized: Invalid user ID",
        })
    }

    // Konversi user_id ke ObjectID
    sellerID, err := primitive.ObjectIDFromHex(userID)
    if err != nil {
        return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
            "message": "Invalid User ID format",
        })
    }

    // Ambil product_id dari parameter
    productID := c.Params("id")
    objectID, err := primitive.ObjectIDFromHex(productID)
    if err != nil {
        return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
            "message": "Invalid Product ID format",
        })
    }

    // Hapus hanya jika produk milik seller
    productCollection := config.MongoClient.Database("ecommerce").Collection("products")
    result, err := productCollection.DeleteOne(context.Background(), bson.M{"_id": objectID, "seller_id": sellerID})
    if err != nil {
        return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
            "message": "Failed to delete product",
            "error":   err.Error(),
        })
    }

    if result.DeletedCount == 0 {
        return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
            "message": "Forbidden: You do not have permission to delete this product",
        })
    }

    return c.JSON(fiber.Map{
        "message": "Product deleted successfully",
        "status":  "success",
    })
}

func DeleteSeller(c *fiber.Ctx) error {
	id := c.Params("id")
	userID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid ID",
		})
	}

	collection := getUserCollection()
	_, err = collection.DeleteOne(context.Background(), bson.M{"_id": userID, "roles": "seller"})
	if err != nil {
		log.Println("Error deleting seller:", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Error deleting seller",
		})
	}

	return c.JSON(fiber.Map{
		"message": "Seller deleted successfully",
	})
}

// CRUD for Users with Both Roles (Customer and Seller)
func GetCustomerSellers(c *fiber.Ctx) error {
	collection := getUserCollection()
	cursor, err := collection.Find(context.Background(), bson.M{"roles": bson.M{"$all": []string{"customer", "seller"}}})
	if err != nil {
		log.Println("Error fetching customer-sellers:", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Error fetching customer-sellers",
		})
	}

	var customerSellers []model.User
	if err = cursor.All(context.Background(), &customerSellers); err != nil {
		log.Println("Error decoding customer-sellers:", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Error decoding customer-sellers",
		})
	}

	return c.JSON(customerSellers)
}

func CreateCustomerSeller(c *fiber.Ctx) error {
	var user model.User
	if err := c.BodyParser(&user); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
		})
	}

	user.Roles = []string{"customer", "seller"}
	user.ID = primitive.NewObjectID()
	collection := getUserCollection()
	_, err := collection.InsertOne(context.Background(), user)
	if err != nil {
		log.Println("Error creating customer-seller:", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Error creating customer-seller",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Customer-seller created successfully",
	})
}

func UpdateCustomerSeller(c *fiber.Ctx) error {
	id := c.Params("id")
	userID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid ID",
		})
	}

	var updates bson.M
	if err := c.BodyParser(&updates); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
		})
	}

	collection := getUserCollection()
	_, err = collection.UpdateOne(context.Background(), bson.M{"_id": userID, "roles": bson.M{"$all": []string{"customer", "seller"}}}, bson.M{"$set": updates})
	if err != nil {
		log.Println("Error updating customer-seller:", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Error updating customer-seller",
		})
	}

	return c.JSON(fiber.Map{
		"message": "Customer-seller updated successfully",
	})
}

func DeleteCustomerSeller(c *fiber.Ctx) error {
	id := c.Params("id")
	userID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid ID",
		})
	}

	collection := getUserCollection()
	_, err = collection.DeleteOne(context.Background(), bson.M{"_id": userID, "roles": bson.M{"$all": []string{"customer", "seller"}}})
	if err != nil {
		log.Println("Error deleting customer-seller:", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Error deleting customer-seller",
		})
	}

	return c.JSON(fiber.Map{
		"message": "Customer-seller deleted successfully",
	})
}

// Suspend User Account
func SuspendUser(c *fiber.Ctx) error {
	id := c.Params("id")
	userID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid ID",
		})
	}

	collection := getUserCollection()
	_, err = collection.UpdateOne(context.Background(), bson.M{"_id": userID}, bson.M{"$set": bson.M{"suspended": true}})
	if err != nil {
		log.Println("Error suspending user:", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Error suspending user",
		})
	}

	return c.JSON(fiber.Map{
		"message": "User account suspended successfully",
	})
}

func UnsuspendUser(c *fiber.Ctx) error {
	id := c.Params("id")
	userID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid ID",
		})
	}

	collection := getUserCollection()
	_, err = collection.UpdateOne(context.Background(), bson.M{"_id": userID}, bson.M{"$set": bson.M{"suspended": false}})
	if err != nil {
		log.Println("Error unsuspending user:", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Error unsuspending user",
		})
	}

	return c.JSON(fiber.Map{
		"message": "User account unsuspended successfully",
	})
}
func SuspendSeller(c *fiber.Ctx) error {
	sellerID := c.Params("id")
	log.Println("Suspend request received for seller ID:", sellerID)

	objectID, err := primitive.ObjectIDFromHex(sellerID)
	if err != nil {
		log.Println("Invalid seller ID format:", sellerID)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid seller ID format",
		})
	}

	collection := getUserCollection()
	filter := bson.M{"_id": objectID, "roles": bson.M{"$all": []string{"seller"}}}
	update := bson.M{"$set": bson.M{"suspended": true}}

	result, err := collection.UpdateOne(context.Background(), filter, update)
	if err != nil {
		log.Println("Error suspending seller:", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to suspend seller",
		})
	}

	if result.ModifiedCount == 0 {
		log.Println("Seller not found or already suspended:", sellerID)
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"message": "Seller not found or already suspended",
		})
	}

	log.Println("Seller suspended successfully:", sellerID)
	return c.JSON(fiber.Map{
		"message": "Seller suspended successfully",
	})
}

func UnsuspendSeller(c *fiber.Ctx) error {
	sellerID := c.Params("id")
	log.Println("Received unsuspend request for seller ID:", sellerID)

	objectID, err := primitive.ObjectIDFromHex(sellerID)
	if err != nil {
		log.Println("Invalid ObjectID:", sellerID)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid seller ID format",
		})
	}

	collection := getUserCollection()
	filter := bson.M{"_id": objectID, "roles": bson.M{"$all": []string{"seller"}}}
	update := bson.M{"$set": bson.M{"suspended": false}}

	result, err := collection.UpdateOne(context.Background(), filter, update)
	if err != nil {
		log.Println("Database error:", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to unsuspend seller",
		})
	}

	if result.ModifiedCount == 0 {
		log.Println("No matching seller found or already unsuspended:", sellerID)
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"message": "Seller not found or already unsuspended",
		})
	}

	log.Println("Seller unsuspended successfully:", sellerID)
	return c.JSON(fiber.Map{
		"message": "Seller unsuspended successfully",
	})
}
