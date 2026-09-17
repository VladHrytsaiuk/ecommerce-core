-- A full-order refund can be initiated only once. Multiple historic/closed
-- RMAs remain queryable, while concurrent new/approved/received requests for
-- the same Order are rejected before any gateway call is made.
CREATE UNIQUE INDEX return_requests_one_active_order_idx
    ON return_requests (order_id)
    WHERE status IN ('new', 'approved', 'received');
