package controllers

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"

	"ccrt_sever/models"
)

func answerPayloadForScoring(input MMSEAnswerInput) json.RawMessage {
	if len(bytes.TrimSpace(input.AnswerPayload)) > 0 {
		return input.AnswerPayload
	}
	return input.UserAnswer
}

func compactJSONText(raw json.RawMessage) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return ""
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, trimmed); err == nil {
		return compact.String()
	}
	return strings.TrimSpace(string(trimmed))
}

func legacyUserAnswerText(input MMSEAnswerInput) string {
	if len(bytes.TrimSpace(input.UserAnswer)) > 0 {
		return strings.TrimSpace(string(bytes.TrimSpace(input.UserAnswer)))
	}
	if text := extractText(answerPayloadForScoring(input)); strings.TrimSpace(text) != "" {
		return strings.TrimSpace(text)
	}
	return compactJSONText(answerPayloadForScoring(input))
}

func validateArtifactRefs(refs []MMSEArtifactRefInput) error {
	for _, ref := range refs {
		if strings.TrimSpace(ref.Kind) == "" {
			return errors.New("artifact kind is required")
		}
		if strings.TrimSpace(ref.ArtifactKey) == "" {
			return errors.New("artifact key is required")
		}
	}
	return nil
}

func buildScaleAnswerArtifacts(
	userID uint,
	questionID uint,
	input MMSEAnswerInput,
) []models.ScaleAnswerArtifact {
	if len(input.ArtifactRefs) == 0 {
		return nil
	}
	artifacts := make([]models.ScaleAnswerArtifact, 0, len(input.ArtifactRefs))
	for _, ref := range input.ArtifactRefs {
		artifact := models.ScaleAnswerArtifact{
			UserID:       userID,
			QuestionID:   questionID,
			Kind:         strings.TrimSpace(ref.Kind),
			ArtifactKey:  strings.TrimSpace(ref.ArtifactKey),
			ReviewStatus: strings.TrimSpace(ref.ReviewStatus),
			Meta:         compactJSONText(ref.Meta),
		}
		if artifact.ReviewStatus == "" {
			artifact.ReviewStatus = "pending"
		}
		if ref.AutoScore != nil {
			artifact.AutoScore = *ref.AutoScore
		}
		if ref.Confidence != nil {
			artifact.Confidence = *ref.Confidence
		}
		artifacts = append(artifacts, artifact)
	}
	return artifacts
}
