package process

import (
	domainattachment "go.abhg.dev/cardamom/internal/attachment"
	repositoryattachment "go.abhg.dev/cardamom/internal/repository/attachment"
)

func provideAttachmentService(
	runtime *namespaceRuntime,
) (*domainattachment.Service, error) {
	return runtime.attachmentService()
}

func (r *namespaceRuntime) attachmentService() (*domainattachment.Service, error) {
	repository, err := repositoryattachment.New(r.store, repositoryattachment.Config{
		StoreDirectory: r.directory,
		Clock:          r.clock,
		Entropy:        r.entropy,
	})
	if err != nil {
		return nil, err
	}
	return domainattachment.NewService(domainattachment.ServiceConfig{
		Repository: repository, Configuration: r.configuration,
	}), nil
}
