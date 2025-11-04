package domain

// ZitadelWebhookRequest - структура запроса от Zitadel Actions V2
type ZitadelWebhookRequest struct {
	FullMethod string                 `json:"fullMethod"`
	Request    map[string]interface{} `json:"request"`
	Context    map[string]interface{} `json:"context"`
}

// ZitadelWebhookResponse - стандартный ответ на webhook
type ZitadelWebhookResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// ExtractPhoneNumber - извлекает номер телефона из webhook request
func (w *ZitadelWebhookRequest) ExtractPhoneNumber() (string, bool) {
	// Пробуем извлечь из Request["human"]["phone"]["phone"]
	if human, ok := w.Request["human"].(map[string]interface{}); ok {
		if phone, ok := human["phone"].(map[string]interface{}); ok {
			if phoneStr, ok := phone["phone"].(string); ok {
				return phoneStr, true
			}
		}
	}
	return "", false
}

// ExtractUsername - извлекает username из webhook request
func (w *ZitadelWebhookRequest) ExtractUsername() (string, bool) {
	if username, ok := w.Request["username"].(string); ok {
		return username, true
	}
	return "", false
}

// ExtractOrganizationID - извлекает organization ID из webhook request
func (w *ZitadelWebhookRequest) ExtractOrganizationID() (string, bool) {
	if orgID, ok := w.Request["organizationId"].(string); ok {
		return orgID, true
	}
	return "", false
}
