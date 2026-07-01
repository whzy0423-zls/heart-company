package sms

import (
	"context"
	"fmt"

	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	dysmsapi "github.com/alibabacloud-go/dysmsapi-20170525/v5/client"
	"github.com/alibabacloud-go/tea/dara"
)

type AliyunSender struct {
	client     *dysmsapi.Client
	signName   string
	templateID string
}

func NewAliyunSender(accessKeyID, accessKeySecret, signName, templateID string) (*AliyunSender, error) {
	config := &openapi.Config{
		AccessKeyId:     dara.String(accessKeyID),
		AccessKeySecret: dara.String(accessKeySecret),
		RegionId:        dara.String("cn-hangzhou"),
	}
	c, err := dysmsapi.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("sms: create aliyun client: %w", err)
	}
	return &AliyunSender{
		client:     c,
		signName:   signName,
		templateID: templateID,
	}, nil
}

func (s *AliyunSender) Send(ctx context.Context, phone, code string) error {
	req := &dysmsapi.SendSmsRequest{
		PhoneNumbers:  dara.String(phone),
		SignName:      dara.String(s.signName),
		TemplateCode:  dara.String(s.templateID),
		TemplateParam: dara.String(fmt.Sprintf(`{"code":"%s"}`, code)),
	}
	resp, err := s.client.SendSms(req)
	if err != nil {
		return fmt.Errorf("sms: send failed: %w", err)
	}
	if resp.Body == nil || resp.Body.Code == nil || *resp.Body.Code != "OK" {
		msg := "unknown error"
		if resp.Body != nil && resp.Body.Message != nil {
			msg = *resp.Body.Message
		}
		return fmt.Errorf("sms: provider rejected: %s", msg)
	}
	return nil
}
