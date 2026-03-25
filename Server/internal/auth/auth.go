package auth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
	
	"hashtasker-go/internal/database"
	"hashtasker-go/internal/models"
	
	"github.com/go-ldap/ldap/v3"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type AuthService struct {
	ldapConfig models.LDAPConfig
}

func NewAuthService(ldapConfig models.LDAPConfig) *AuthService {
	return &AuthService{
		ldapConfig: ldapConfig,
	}
}

func (a *AuthService) Authenticate(username, password string) (*models.User, error) {
	if a.ldapConfig.Enabled {
		user, err := a.authenticateLDAP(username, password)
		if err == nil {
			return user, nil
		}
	}
	
	return a.authenticateLocal(username, password)
}

func (a *AuthService) authenticateLDAP(username, password string) (*models.User, error) {
	conn, err := ldap.Dial("tcp", fmt.Sprintf("%s:%d", a.ldapConfig.Host, a.ldapConfig.Port))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to LDAP: %v", err)
	}
	defer conn.Close()

	if err := conn.Bind(a.ldapConfig.BindDN, a.ldapConfig.BindPass); err != nil {
		return nil, fmt.Errorf("failed to bind to LDAP: %v", err)
	}

	searchRequest := ldap.NewSearchRequest(
		a.ldapConfig.BaseDN,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		fmt.Sprintf(a.ldapConfig.UserFilter, username),
		[]string{"dn", "cn", "mail", "memberOf"},
		nil,
	)

	sr, err := conn.Search(searchRequest)
	if err != nil {
		return nil, fmt.Errorf("LDAP search failed: %v", err)
	}

	if len(sr.Entries) == 0 {
		return nil, fmt.Errorf("user not found in LDAP")
	}

	userDN := sr.Entries[0].DN
	if err := conn.Bind(userDN, password); err != nil {
		return nil, fmt.Errorf("invalid LDAP credentials")
	}

	isAdmin := false
	if a.ldapConfig.AdminGroup != "" {
		for _, attr := range sr.Entries[0].GetAttributeValues("memberOf") {
			if attr == a.ldapConfig.AdminGroup {
				isAdmin = true
				break
			}
		}
	}

	email := ""
	if len(sr.Entries[0].GetAttributeValues("mail")) > 0 {
		email = sr.Entries[0].GetAttributeValues("mail")[0]
	}

	var user models.User
	result := database.DB.Where("username = ?", username).First(&user)
	if result.Error == gorm.ErrRecordNotFound {
		user = models.User{
			Username: username,
			Email:    email,
			IsAdmin:  isAdmin,
			IsLocal:  false,
		}
		if err := database.DB.Create(&user).Error; err != nil {
			return nil, fmt.Errorf("failed to create user: %v", err)
		}
	} else {
		user.Email = email
		user.IsAdmin = isAdmin
		database.DB.Save(&user)
	}

	return &user, nil
}

func (a *AuthService) authenticateLocal(username, password string) (*models.User, error) {
	var user models.User
	result := database.DB.Where("username = ? AND is_local = ?", username, true).First(&user)
	if result.Error != nil {
		return nil, fmt.Errorf("invalid credentials")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		return nil, fmt.Errorf("invalid credentials")
	}

	return &user, nil
}

func (a *AuthService) CreateSession(userID uint) (*models.Session, error) {
	sessionID, err := generateSessionID()
	if err != nil {
		return nil, err
	}

	session := models.Session{
		ID:        sessionID,
		UserID:    userID,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}

	if err := database.DB.Create(&session).Error; err != nil {
		return nil, fmt.Errorf("failed to create session: %v", err)
	}

	return &session, nil
}

func (a *AuthService) ValidateSession(sessionID string) (*models.User, error) {
	var session models.Session
	result := database.DB.Preload("User").Where("id = ? AND expires_at > ?", sessionID, time.Now()).First(&session)
	if result.Error != nil {
		return nil, fmt.Errorf("invalid or expired session")
	}

	return &session.User, nil
}

func (a *AuthService) DeleteSession(sessionID string) error {
	return database.DB.Where("id = ?", sessionID).Delete(&models.Session{}).Error
}

func generateSessionID() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}