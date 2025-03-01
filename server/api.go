package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/mattermost/mattermost-server/v5/plugin"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/translate"
	"github.com/mattermost/mattermost-server/v5/model"
)

// APIErrorResponse as standard response error
type APIErrorResponse struct {
	ID         string `json:"id"`
	Message    string `json:"message"`
	StatusCode int    `json:"status_code"`
}

func writeAPIError(w http.ResponseWriter, err *APIErrorResponse) {
	b, _ := json.Marshal(err)
	w.WriteHeader(err.StatusCode)
	w.Write(b)
}

func (p *Plugin) ServeHTTP(c *plugin.Context, w http.ResponseWriter, r *http.Request) {
	if err := p.IsValid(); err != nil {
		http.Error(w, "This plugin is not configured.", http.StatusNotImplemented)
	}

	w.Header().Set("Content-Type", "application/json")

	switch path := r.URL.Path; path {
	case "/api/go":
		p.getGo(w, r)
	case "/api/get_info":
		p.getInfo(w, r)
	case "/api/set_info":
		p.setInfo(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (p *Plugin) translateText(text, sourceLang, targetLang string) (string, *model.AppError) {
	configuration := p.getConfiguration()
	sess := session.Must(session.NewSession())
	creds := credentials.NewStaticCredentials(configuration.AWSAccessKeyID, configuration.AWSSecretAccessKey, "")
	_, awsErr := creds.Get()
	if awsErr != nil {
		return "", model.NewAppError("translateText", "BadCredentials", nil, "Invalid AWS credentials", http.StatusForbidden)
	}

	svc := translate.New(sess, aws.NewConfig().WithCredentials(creds).WithRegion(configuration.AWSRegion))

	input := translate.TextInput{
		SourceLanguageCode: &sourceLang,
		TargetLanguageCode: &targetLang,
		Text:               &text,
	}

	output, awsErr := svc.Text(&input)
	if awsErr != nil {
		return "", model.NewAppError("translateText", "TranslationFailed", nil, "Translation API error", http.StatusInternalServerError)
	}

	return *output.TranslatedText, nil
}

func (p *Plugin) getGo(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("Mattermost-User-ID")
	if userID == "" {
		http.Error(w, "Not authorized to translate post", http.StatusUnauthorized)
		return
	}

	postID := r.URL.Query().Get("post_id")
	source := r.URL.Query().Get("source")
	target := r.URL.Query().Get("target")

	post, err := p.API.GetPost(postID)
	if err != nil {
		http.Error(w, "No post to translate", http.StatusBadRequest)
		return
	}

	// 🔹 言語が "auto" の場合は自動検出
	if source == "auto" {
		detected, err := p.detectLanguage(post.Message)
		if err != nil {
			http.Error(w, "Language detection failed", http.StatusBadRequest)
			return
		}
		source = detected
	}

	translatedText, err := p.translateText(post.Message, source, target)
	if err != nil {
		http.Error(w, "Translation failed", http.StatusBadRequest)
		return
	}

	translated := TranslatedMessage{
		ID:             postID + source + target + strconv.FormatInt(post.UpdateAt, 10),
		PostID:         postID,
		SourceLanguage: source,
		SourceText:     post.Message,
		TargetLanguage: target,
		TranslatedText: translatedText,
		UpdateAt:       post.UpdateAt,
	}

	resp, _ := json.Marshal(translated)
	w.Write(resp)
}

func (p *Plugin) getInfo(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("Mattermost-User-ID")
	if userID == "" {
		// silently return as user is probably not logged in
		return
	}

	info, err := p.getUserInfo(userID)
	if err != nil {
		// silently return as user may not have activated the autotranslation
		return
	}

	resp, _ := json.Marshal(info)
	w.Write(resp)
}

func (p *Plugin) setInfo(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("Mattermost-User-ID")
	if userID == "" {
		http.Error(w, "Not authorized to set info", http.StatusUnauthorized)
		return
	}

	var info *UserInfo
	json.NewDecoder(r.Body).Decode(&info)
	if info == nil {
		http.Error(w, "Invalid parameter: info", http.StatusBadRequest)
		return
	}

	if err := info.IsValid(); err != nil {
		http.Error(w, fmt.Sprintf("Invalid info: %s", err.Error()), http.StatusBadRequest)
		return
	}

	if info.UserID != userID {
		http.Error(w, "Invalid parameter: user mismatch", http.StatusBadRequest)
		return
	}

	err := p.setUserInfo(info)
	if err != nil {
		http.Error(w, "Failed to set info", http.StatusBadRequest)
		return
	}

	resp, _ := json.Marshal(info)
	w.Write(resp)
}
