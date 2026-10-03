package orcaworker

import (
	"context"
	"errors"
)

func ReadDelivery(ctx context.Context, call MetaCall, coordinator, runtime, runID string) (Inbox, error) {
	v, err := CheckInbox(ctx, call, coordinator, runID)
	if err != nil {
		return v, err
	}
	if v.RuntimeID != runtime {
		return v, errors.New("Orca coordinator runtime changed")
	}
	if v.RunID != runID || v.ConnectionLost || len(v.Messages) > 0 && v.DeliveryID == "" {
		return v, errors.New("Orca inbox delivery identity is unverified")
	}
	return v, nil
}
func Settlement(ctx context.Context, call MetaCall, identity DispatchIdentity, outcome string) (string, error) {
	status, err := VerifySettledDispatch(ctx, call, identity)
	if err != nil {
		return "", err
	}
	if outcome == "succeeded" && status != "completed" || outcome == "failed" && status != "failed" {
		return "", errors.New("Orca worker_done and Dispatch settlement disagree")
	}
	if outcome != "succeeded" && outcome != "failed" {
		return "", errors.New("Orca worker_done has no verified outcome")
	}
	return outcome, nil
}
func DeliveryAcknowledged(ctx context.Context, call MetaCall, coordinator, runtime, runID, deliveryID string) (bool, error) {
	v, err := ReadDelivery(ctx, call, coordinator, runtime, runID)
	if err != nil {
		return false, err
	}
	return v.DeliveryID != deliveryID, nil
}
func InboxDrained(ctx context.Context, call MetaCall, coordinator, runtime, runID string) error {
	v, err := ReadDelivery(ctx, call, coordinator, runtime, runID)
	if err != nil {
		return err
	}
	if v.DeliveryID != "" || len(v.Messages) != 0 {
		return errors.New("Orca coordinator inbox is not drained after delivery")
	}
	return nil
}
