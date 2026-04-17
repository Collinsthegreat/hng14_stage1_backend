package migrations

import _ "embed"

//go:embed 001_create_profiles.sql
var CreateProfilesSQL string
