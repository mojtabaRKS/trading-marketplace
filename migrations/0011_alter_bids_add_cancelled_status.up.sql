ALTER TABLE bids DROP CONSTRAINT chk_bid_status;
ALTER TABLE bids ADD CONSTRAINT chk_bid_status
    CHECK (status IN ('active','released','won','cancelled'));
