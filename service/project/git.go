package project

import (
	"github.com/AlecAivazis/survey/v2"
)

func (that *Service) InputGitScr() (string, error) {
	var qs = []*survey.Question{
		{
			Name:   "GitSrc",
			Prompt: &survey.Input{Message: "请输入项目的外部 Git 仓库地址（支持 HTTPS、SSH 或 scp 风格地址）: "},
		},
	}
	answers := struct {
		GitSrc string
	}{}
	// perform the questions
	err := survey.Ask(qs, &answers)
	if err != nil {
		return "", err
	}
	return answers.GitSrc, nil
}
