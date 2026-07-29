package process

import (
	"go.abhg.dev/cardamom/internal/cli"
	"go.abhg.dev/cardamom/internal/mail"
	repositorylease "go.abhg.dev/cardamom/internal/repository/lease"
	repositorymail "go.abhg.dev/cardamom/internal/repository/mail"
)

func (r *namespaceRuntime) mailService() *mail.Service {
	repository := repositorymail.New(r.store, repositorymail.Config{
		Clock: r.clock, Entropy: r.entropy,
	})
	return mail.NewService(repository, r.clock)
}

func (r *namespaceRuntime) leaseOperations() *repositorylease.Repository {
	return repositorylease.New(r.store, r.clock)
}

func provideMailService(
	runtime *namespaceRuntime,
) *mail.Service {
	return runtime.mailService()
}

func provideMailOperations(service *mail.Service) cli.MailOperations {
	return service
}

func provideLeaseOperations(runtime *namespaceRuntime) cli.LeaseOperations {
	return runtime.leaseOperations()
}
