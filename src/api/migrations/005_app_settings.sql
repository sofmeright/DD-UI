-- Application settings tables (key-value store pattern)
CREATE TABLE IF NOT EXISTS app_settings (
    key VARCHAR(255) PRIMARY KEY,
    value TEXT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS host_settings (
    host_name VARCHAR(255) PRIMARY KEY,
    auto_apply_override BOOLEAN DEFAULT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS group_settings (
    group_name VARCHAR(255) PRIMARY KEY,
    auto_apply_override BOOLEAN DEFAULT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Triggers
CREATE TRIGGER app_settings_updated_at 
    BEFORE UPDATE ON app_settings 
    FOR EACH ROW 
    EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER host_settings_updated_at 
    BEFORE UPDATE ON host_settings 
    FOR EACH ROW 
    EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER group_settings_updated_at 
    BEFORE UPDATE ON group_settings 
    FOR EACH ROW 
    EXECUTE FUNCTION set_updated_at();