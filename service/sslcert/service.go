package sslcert

import (
	"galaxy/pkg/galaxycfg"
	"galaxy/pkg/httpclient"
)

type Service struct {
	cfgFlags *galaxycfg.ConfigFlags
	client   *httpclient.HttpClient
}
type CertificateList struct {
	Total uint32         `json:"total"`
	Data  []*Certificate `json:"data"`
}
type Certificate struct {
	ID                 uint32 `json:"id"`
	Title              string `json:"title"`
	CommonName         string `json:"common_name"`
	Issuer             string `json:"issuer"`
	SignatureAlgorithm string `json:"signature_algorithm"`
	Fingerprint        string `json:"fingerprint_sha256"`
	ValidFrom          int64  `json:"valid_from"`
	ValidTo            int64  `json:"valid_to"`
}

func (that *Service) Certificates(orgID uint32) (*CertificateList, error) {
	var rsp *CertificateList
	err := that.client.Get("/resources/certificates", map[string]interface{}{"org": orgID}, &rsp)
	return rsp, err
}

func (that *Service) Import(orgID uint32, title, certificatePEM, privateKeyPEM string) (*Certificate, error) {
	var rsp struct {
		Certificate *Certificate `json:"certificate"`
	}
	err := that.client.Post("/resources/certificates/import", map[string]interface{}{
		"org": orgID, "title": title, "source": "manual",
		"certificate_pem": certificatePEM, "private_key_pem": privateKeyPEM,
	}, &rsp)
	if err != nil {
		return nil, err
	}
	return rsp.Certificate, nil
}

func (that *Service) DeleteCertificate(orgID, certificateID uint32) error {
	return that.client.Delete("/resources/certificates", map[string]interface{}{"org": orgID, "certificate_id": certificateID}, nil)
}

func (that *Service) Find(orgID uint32, keyword string) ([]*Certificate, error) {
	var rsp *CertificateList
	if err := that.client.Get("/resources/certificates", map[string]interface{}{"org": orgID, "keyword": keyword, "pagesize": 100}, &rsp); err != nil {
		return nil, err
	}
	return rsp.Data, nil
}

func NewService(cfgFlags *galaxycfg.ConfigFlags) *Service {
	return &Service{
		cfgFlags: cfgFlags,
		client:   httpclient.NewHttpClient(cfgFlags),
	}
}
