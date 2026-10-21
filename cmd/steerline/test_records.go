package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/erranliu/SteerLine/internal/app"
	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/product/applicationclient"
	"github.com/erranliu/SteerLine/internal/testmanagement"
	tm "github.com/erranliu/SteerLine/internal/testmanagement/model"
	"os"
	"path/filepath"
	"time"
)

func runTestRegister(args []string) error {
	fs := flag.NewFlagSet("tests register", flag.ContinueOnError)
	project := fs.String("project", "", "configured project")
	dir := fs.String("report-dir", "", "absolute directory of managed reports")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 || *project == "" || !filepath.IsAbs(*dir) {
		return errors.New("register requires -project and absolute -report-dir")
	}
	items, err := testmanagement.ReadRegistrationDirectory(*dir)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	p, connectErr := app.OpenApplicationProject(ctx, *project, "")
	if p != nil {
		defer p.Close()
	}
	receipts, err := testmanagement.RegisterFiles(*dir, items, func(in tm.RegisterRecord, requestID string) testmanagement.RegistrationResult {
		callErr := connectErr
		var result json.RawMessage
		if callErr == nil {
			result, callErr = p.RegisterTestRecord(ctx, api.TestRecordRegistration(in), requestID)
		}
		outcome := testmanagement.RegistrationResult{Status: applicationclient.RecordRegistrationStatus(callErr), Result: result}
		if callErr != nil {
			outcome.Diagnostic = callErr.Error()
		}
		return outcome
	})
	if err != nil {
		return err
	}
	failed := false
	for _, receipt := range receipts {
		failed = failed || receipt.Status != "registered"
	}
	_ = json.NewEncoder(os.Stdout).Encode(receipts)
	if failed {
		return fmt.Errorf("one or more test records were not registered; original reports retained")
	}
	return nil
}
