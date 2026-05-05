-- Composite indexes for common filter combinations.
CREATE INDEX IF NOT EXISTS idx_profiles_gender_country
    ON profiles(LOWER(gender), LOWER(country_id));

CREATE INDEX IF NOT EXISTS idx_profiles_gender_age_group
    ON profiles(LOWER(gender), LOWER(age_group));

CREATE INDEX IF NOT EXISTS idx_profiles_country_age
    ON profiles(LOWER(country_id), age);

CREATE INDEX IF NOT EXISTS idx_profiles_age_range
    ON profiles(age);

CREATE INDEX IF NOT EXISTS idx_profiles_gender_prob
    ON profiles(gender_probability);

CREATE INDEX IF NOT EXISTS idx_profiles_country_prob
    ON profiles(country_probability);

-- Covering index for list queries.
CREATE INDEX IF NOT EXISTS idx_profiles_covering
    ON profiles(LOWER(gender), LOWER(country_id), LOWER(age_group), age)
    INCLUDE (id, name, gender_probability, country_name, country_probability, created_at);
