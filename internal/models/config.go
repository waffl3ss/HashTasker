package models

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	LDAP     LDAPConfig     `yaml:"ldap"`
	Admin    AdminConfig    `yaml:"admin"`
}

type ServerConfig struct {
	Host          string `yaml:"host" default:"0.0.0.0"`
	Port          int    `yaml:"port" default:"8080"`
	SessionKey    string `yaml:"session_key"`
	UploadPath    string `yaml:"upload_path" default:"./uploads"`
	StaticPath    string `yaml:"static_path" default:"./web/static"`
	TemplatePath  string `yaml:"template_path" default:"./web/templates"`
	WordlistsPath string `yaml:"wordlists_path" default:"./wordlists"`
	RulesetsPath  string `yaml:"rulesets_path" default:"./rulesets"`
	HashmodesPath string `yaml:"hashmodes_path" default:"./hashmodes.txt"`
}

type DatabaseConfig struct {
	Host     string `yaml:"host" default:"localhost"`
	Port     int    `yaml:"port" default:"3306"`
	User     string `yaml:"user" default:"hashtasker"`
	Password string `yaml:"password"`
	Database string `yaml:"database" default:"hashtasker"`
}

type LDAPConfig struct {
	Enabled    bool   `yaml:"enabled" default:"false"`
	Host       string `yaml:"host"`
	Port       int    `yaml:"port" default:"389"`
	BindDN     string `yaml:"bind_dn"`
	BindPass   string `yaml:"bind_pass"`
	BaseDN     string `yaml:"base_dn"`
	UserFilter string `yaml:"user_filter" default:"(uid=%s)"`
	AdminGroup string `yaml:"admin_group"`
}

type AdminConfig struct {
	Username string `yaml:"username" default:"admin"`
	Password string `yaml:"password" default:"admin123"`
}

type WordlistOption struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Size     int64  `json:"size"`
	Modified string `json:"modified"`
}

type RulesetOption struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Rules    int    `json:"rules"`
	Modified string `json:"modified"`
}

type HashModeOption struct {
	Mode        int    `json:"mode"`
	Name        string `json:"name"`
	Category    string `json:"category"`
	Description string `json:"description"`
}