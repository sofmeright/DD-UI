-- src/api/migrations/007_indexes.sql
-- Helpful indexes
CREATE INDEX IF NOT EXISTS idx_stacks_host_id     ON stacks(host_id);
CREATE INDEX IF NOT EXISTS idx_containers_host_id ON containers(host_id);
CREATE INDEX IF NOT EXISTS idx_containers_stack_id ON containers(stack_id);