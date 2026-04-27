package migrations

import _ "embed"

//go:embed 001_create_profiles.sql
var CreateProfilesSQL string

//go:embed 002_add_country_name.sql
var AddCountryNameSQL string

//go:embed 003_create_users_tokens.sql
var CreateUsersTokensSQL string
