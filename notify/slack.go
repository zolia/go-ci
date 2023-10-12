/*
 * Copyright (c) 2021. Monimoto, UAB. All Rights Reserved.
 *
 * This software contains the intellectual property of Monimoto, UAB. Use of this software and the intellectual
 * property contained therein is expressly limited to the terms and conditions of the License Agreement under
 * which it is provided by or on behalf of Monimoto, UAB.
 */

package notify

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/slack-go/slack"
)

// Color is a Slack message color
type Color string

const (
	Good Color = "good"
	Bad  Color = "danger"
)

// DeploymentMessage is a message sent to Slack on deployment
type DeploymentMessage struct {
	SlackURL   string
	Env        string
	Repo       string
	Error      string
	Author     string
	AuthorLink string
	Color      Color
}

// SlackDeployment sends a Slack message on deployment
func SlackDeployment(msg DeploymentMessage) error {
	branchName, log, err := getBranchAndLog()
	if err != nil {
		return fmt.Errorf("failed to get branch and log: %w", err)
	}

	title := fmt.Sprintf("Deployed: %s env", msg.Env)
	text := "Redeployment."
	if strings.TrimSpace(log) != "" {
		text = fmt.Sprintf("Log: \n%s", log)
	}
	if msg.Color == Bad {
		title = fmt.Sprintf("Failed to deploy: %s env", msg.Env)
		text = fmt.Sprintf("Error: %s\n", msg.Error)
	}
	attachment := slack.Attachment{
		Color:      string(msg.Color),
		AuthorName: msg.Author,
		AuthorLink: msg.AuthorLink,
		Title:      title,
		Pretext:    fmt.Sprintf("Branch: %s ", strings.Replace(branchName, "refs/heads/", "", 1)),
		Text:       text,
	}

	err = slack.PostWebhook(msg.SlackURL, &slack.WebhookMessage{
		Attachments: []slack.Attachment{attachment},
	})
	if err != nil {
		return fmt.Errorf("failed to send Slack notification: %w", err)
	}

	return nil
}

func getBranchAndLog() (string, string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", "", err
	}

	repo, err := git.PlainOpen(dir)
	if err != nil {
		return "", "", err
	}

	h, err := repo.Head()
	if err != nil {
		return "", "", err
	}

	since := time.Now().AddDate(0, 0, -1)
	cIter, err := repo.Log(&git.LogOptions{From: h.Hash(), Since: &since})
	if err != nil {
		return "", "", err
	}

	var logEntries []string
	f := func(c *object.Commit) error {
		logEntries = append(logEntries, c.Message)

		return nil
	}
	err = cIter.ForEach(f)
	if err != nil {
		return "", "", fmt.Errorf("failed to iterate over commits: %w", err)
	}

	logs := strings.Join(logEntries, "\n - ")

	return h.Name().String(), logs, nil
}
