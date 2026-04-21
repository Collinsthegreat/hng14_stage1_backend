-- Migration: add country_name if not exists
ALTER TABLE profiles ADD COLUMN IF NOT EXISTS country_name VARCHAR NOT NULL DEFAULT '';
