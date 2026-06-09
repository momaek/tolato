package notify

import "github.com/momaek/tolato/server/internal/model"

const ctJSON = "application/json"

// presetSpec returns the default webhook spec for a built-in preset. Users only
// fill the channel's Params (and reference a token source for two-step ones);
// these defaults supply method/url/body/success_check. Returns (zero, false)
// for unknown or "custom" presets.
func presetSpec(preset string) (model.NotifyWebhookSpec, bool) {
	switch preset {
	case "telegram":
		return model.NotifyWebhookSpec{
			Method:       "POST",
			URL:          "https://api.telegram.org/bot{{bot_token}}/sendMessage",
			Headers:      map[string]string{"Content-Type": ctJSON},
			BodyTemplate: `{"chat_id":"{{chat_id}}","text":"{{message}}"}`,
			SuccessCheck: &model.NotifyJSONMatch{JSONPath: "ok", Values: []any{true}},
		}, true
	case "discord":
		// Incoming webhook; token already embedded in {{url}}. Returns 204.
		return model.NotifyWebhookSpec{
			Method:       "POST",
			URL:          "{{url}}",
			Headers:      map[string]string{"Content-Type": ctJSON},
			BodyTemplate: `{"content":"{{message}}"}`,
		}, true
	case "slack":
		// Incoming webhook; returns 200 with the literal body "ok".
		return model.NotifyWebhookSpec{
			Method:       "POST",
			URL:          "{{url}}",
			Headers:      map[string]string{"Content-Type": ctJSON},
			BodyTemplate: `{"text":"{{message}}"}`,
		}, true
	case "wecom":
		return model.NotifyWebhookSpec{
			Method:       "POST",
			URL:          "https://qyapi.weixin.qq.com/cgi-bin/message/send?access_token={{token}}",
			Headers:      map[string]string{"Content-Type": ctJSON},
			BodyTemplate: `{"touser":"@all","msgtype":"text","agentid":{{agentid}},"text":{"content":"{{message}}"}}`,
			SuccessCheck: &model.NotifyJSONMatch{JSONPath: "errcode", Values: []any{0}},
		}, true
	case "feishu":
		return model.NotifyWebhookSpec{
			Method:       "POST",
			URL:          "https://open.feishu.cn/open-apis/im/v1/messages?receive_id_type=open_id",
			Headers:      map[string]string{"Content-Type": ctJSON, "Authorization": "Bearer {{token}}"},
			BodyTemplate: `{"receive_id":"{{receive_id}}","msg_type":"text","content":"{\"text\":\"{{message}}\"}"}`,
			SuccessCheck: &model.NotifyJSONMatch{JSONPath: "code", Values: []any{0}},
		}, true
	}
	return model.NotifyWebhookSpec{}, false
}

// PresetTokenSource returns a default token-exchange config for a preset, for
// the UI to prefill (the user only fills Params like corpid/secret). Returns
// (zero, false) for presets that need no token exchange. Exported so the
// settings handler can expose presets to the frontend.
func PresetTokenSource(preset string) (model.NotifyTokenSource, bool) {
	switch preset {
	case "wecom":
		return model.NotifyTokenSource{
			Name: "wecom",
			Request: model.NotifyHTTPRequest{
				Method: "GET",
				URL:    "https://qyapi.weixin.qq.com/cgi-bin/gettoken?corpid={{corpid}}&corpsecret={{secret}}",
			},
			Extract: model.NotifyTokenExtract{
				TokenPath:          "access_token",
				ExpiresPath:        "expires_in",
				TTLFallbackSeconds: 7000,
			},
			InvalidateOn: &model.NotifyJSONMatch{JSONPath: "errcode", Values: []any{42001, 40014, 40001}},
		}, true
	case "feishu":
		return model.NotifyTokenSource{
			Name: "feishu",
			Request: model.NotifyHTTPRequest{
				Method:  "POST",
				URL:     "https://open.feishu.cn/open-apis/auth/v3/tenant_access_token/internal",
				Headers: map[string]string{"Content-Type": ctJSON},
				Body:    `{"app_id":"{{app_id}}","app_secret":"{{app_secret}}"}`,
			},
			Extract: model.NotifyTokenExtract{
				TokenPath:          "tenant_access_token",
				ExpiresPath:        "expire",
				TTLFallbackSeconds: 6000,
			},
			InvalidateOn: &model.NotifyJSONMatch{JSONPath: "code", Values: []any{99991663, 99991661, 99991664}},
		}, true
	}
	return model.NotifyTokenSource{}, false
}

// resolveSpec produces the effective webhook spec for a channel: preset
// defaults overlaid with any non-empty fields from the channel's custom Webhook.
func resolveSpec(ch model.NotifyChannel) model.NotifyWebhookSpec {
	spec, _ := presetSpec(ch.Preset)
	if ch.Webhook == nil {
		return spec
	}
	w := ch.Webhook
	if w.Method != "" {
		spec.Method = w.Method
	}
	if w.URL != "" {
		spec.URL = w.URL
	}
	if len(w.Headers) > 0 {
		spec.Headers = w.Headers
	}
	if w.BodyTemplate != "" {
		spec.BodyTemplate = w.BodyTemplate
	}
	if w.SuccessCheck != nil {
		spec.SuccessCheck = w.SuccessCheck
	}
	return spec
}
