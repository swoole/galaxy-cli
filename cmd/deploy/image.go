package deploy

import (
	"fmt"
	"galaxy/service/image"
	"github.com/AlecAivazis/survey/v2"
	"time"
)

func (that *Options) askArtifacts(msg string, artifacts []*image.Artifact, latest bool) (*image.Artifact, error) {

	if len(artifacts) == 1 || latest {
		return artifacts[0], nil
	}
	titles := make([]string, len(artifacts))
	for i, artifact := range artifacts {
		commit := "-"
		branch := "-"
		if artifact.Build != nil {
			branch = artifact.Build.Branch
			commit = artifact.Build.CommitID
			if len(commit) > 8 {
				commit = commit[:8]
			}
		}
		titles[i] = fmt.Sprintf("#%d %s [%s:%s] (%s)", artifact.ID, artifact.Reference, branch, commit, time.Unix(artifact.CreatedAt, 0).Format("2006-01-02 15:04:05"))
	}
	answerIndex := 0
	err := survey.AskOne(&survey.Select{
		Message: msg,
		Options: titles,
		Default: titles[0],
	}, &answerIndex)
	if err != nil {
		return nil, err
	}
	return artifacts[answerIndex], nil
}
