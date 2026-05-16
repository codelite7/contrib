
-- properties: composite (field, id) indexes for cursor-based pagination
CREATE INDEX "idx_order_properties_created_at_id" ON "properties" ("created_at", "id") WHERE (deleted_at IS NULL);
CREATE INDEX "idx_order_properties_description_id" ON "properties" (left("description", 256), "id") WHERE (deleted_at IS NULL);
