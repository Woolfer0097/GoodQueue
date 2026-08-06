-- +goose Up
ALTER TABLE products ADD COLUMN reserved INTEGER NOT NULL DEFAULT 0 CHECK (reserved >= 0);
ALTER TABLE products ADD CONSTRAINT check_reserved_leq_allocatable CHECK (reserved <= allocatable_stock);

ALTER TABLE purchase_rights ADD COLUMN product_id UUID;
UPDATE purchase_rights pr SET product_id = qe.product_id FROM queue_entries qe WHERE qe.ticket_id = pr.queue_ticket_id;
ALTER TABLE purchase_rights ALTER COLUMN product_id SET NOT NULL;
ALTER TABLE purchase_rights ADD CONSTRAINT fk_purchase_rights_product FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE RESTRICT;

CREATE INDEX idx_purchase_rights_product_status ON purchase_rights (product_id, status) WHERE status = 'active';
CREATE INDEX idx_purchase_rights_expires_status ON purchase_rights (expires_at, status) WHERE status = 'active';

-- +goose Down
DROP INDEX IF EXISTS idx_purchase_rights_product_status;
DROP INDEX IF EXISTS idx_purchase_rights_expires_status;
ALTER TABLE purchase_rights DROP CONSTRAINT IF EXISTS fk_purchase_rights_product;
ALTER TABLE purchase_rights DROP COLUMN IF EXISTS product_id;
ALTER TABLE products DROP CONSTRAINT IF EXISTS check_reserved_leq_allocatable;
ALTER TABLE products DROP COLUMN IF EXISTS reserved;
