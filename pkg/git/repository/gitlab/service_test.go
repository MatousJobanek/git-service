package gitlab_test

import (
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"github.com/redhat-developer/git-service/pkg/git"
	"github.com/redhat-developer/git-service/pkg/git/repository/gitlab"
	"github.com/redhat-developer/git-service/pkg/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gogl "github.com/xanzy/go-gitlab"
	"golang.org/x/oauth2"
	"gopkg.in/h2non/gock.v1"
	"math/rand"
	"strings"
	"testing"
)

const (
	pathToTestDir  = "../../../test"
	repoIdentifier = "some-org/some-repo"
	repoURL        = "https://gitlab.com/" + repoIdentifier
	notFound       = `{"message":"404 Project Not Found"}`
)

func TestRepositoryServiceForBothAuthMethodsSuccessful(t *testing.T) {
	// given
	defer gock.OffAll()
	usernamePassword := git.NewUsernamePassword("anonymous", "")
	oauthToken := git.NewOauthToken([]byte("some-token"))
	mockTokenCall(t)

	for _, secret := range []git.Secret{usernamePassword, oauthToken} {
		mockGLCalls(t, repoIdentifier, "master", test.S("pom.xml", "mvnw"), test.S("Java", "Go"))
		source := test.NewGitSource(test.WithURL(repoURL))

		// when
		service, err := gitlab.NewRepoServiceIfMatches()(source, secret)

		// then
		require.NoError(t, err)

		filesInRootDir, err := service.GetListOfFilesInRootDir()
		require.NoError(t, err)
		require.Len(t, filesInRootDir, 2)
		assert.Contains(t, filesInRootDir, "pom.xml")
		assert.Contains(t, filesInRootDir, "mvnw")

		languageList, err := service.GetLanguageList()
		require.NoError(t, err)
		require.Len(t, languageList, 2)
		assert.Contains(t, languageList, "Java")
		assert.Contains(t, languageList, "Go")
	}
}

func TestNewRepoServiceIfMatchesShouldNotMatchWhenSshKey(t *testing.T) {
	// given
	source := test.NewGitSource(test.WithURL("git@gitlab.com:" + repoIdentifier))

	// when
	service, err := gitlab.NewRepoServiceIfMatches()(source,
		git.NewSshKey(test.PrivateWithoutPassphrase(t, pathToTestDir), []byte("")))

	// then
	assert.NoError(t, err)
	assert.Nil(t, service)
}

func TestNewRepoServiceIfMatchesShouldNotMatchWhenGitLabHost(t *testing.T) {
	// given
	source := test.NewGitSource(test.WithURL("gitlab.com/" + repoIdentifier))

	// when
	service, err := gitlab.NewRepoServiceIfMatches()(source, git.NewOauthToken([]byte("some-token")))

	// then
	assert.NoError(t, err)
	assert.Nil(t, service)
}

func TestNewRepoServiceIfMatchesShouldMatchWhenFlavorIsGitHub(t *testing.T) {
	// given
	source := test.NewGitSource(test.WithURL("gitprivatelab.com/"+repoIdentifier), test.WithFlavor("gitlab"))

	// when
	service, err := gitlab.NewRepoServiceIfMatches()(source, git.NewOauthToken([]byte("some-token")))

	// then
	assert.NoError(t, err)
	assert.NotNil(t, service)
}

func TestNewRepoServiceIfMatchesShouldNotFailWhenSsh(t *testing.T) {
	// given
	source := test.NewGitSource(test.WithURL("git@gitlab.com:" + repoIdentifier))

	// when
	service, err := gitlab.NewRepoServiceIfMatches()(source, git.NewOauthToken([]byte("some-token")))

	// then
	assert.NoError(t, err)
	assert.NotNil(t, service)
}

func TestRepositoryServiceForWrongRepo(t *testing.T) {
	// given
	defer gock.OffAll()
	usernamePassword := git.NewUsernamePassword("anonymous", "")
	oauthToken := git.NewOauthToken([]byte("some-token"))
	mockTokenCall(t)

	for _, secret := range []git.Secret{usernamePassword, oauthToken} {
		gock.New("https://gitlab.com").
			Get(fmt.Sprintf("/api/v4/projects/%s/", repoIdentifier)).
			Times(2).
			Reply(404).
			BodyString(notFound)
		source := test.NewGitSource(test.WithURL(repoURL), test.WithRef("dev"))

		// when
		service, err := gitlab.NewRepoServiceIfMatches()(source, secret)

		// then
		require.NoError(t, err)

		filesInRootDir, err := service.GetListOfFilesInRootDir()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Not Found")
		require.Len(t, filesInRootDir, 0)

		languageList, err := service.GetLanguageList()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Not Found")
		require.Len(t, languageList, 0)
	}
}

func mockTokenCall(t *testing.T) {
	token := &oauth2.Token{
		AccessToken: "some-token",
		TokenType:   "bearer",
	}
	bytes, err := json.Marshal(token)
	require.NoError(t, err)
	gock.New("https://gitlab.com").
		Post("/oauth/token").
		Reply(200).
		BodyString(string(bytes))
}

func mockGLCalls(t *testing.T, prjPath, branch string, files, langs test.SliceOfStrings) {
	var treeNodes []gogl.TreeNode
	for _, file := range files() {
		treeNodes = append(treeNodes, gogl.TreeNode{
			ID:   *sha(file),
			Path: file,
			Name: file,
		})
	}

	bytes, err := json.Marshal(treeNodes)
	require.NoError(t, err)

	gock.New("https://gitlab.com").
		Get(fmt.Sprintf("/api/v4/projects/%s/repository/tree", prjPath)).
		MatchParam("ref", branch).
		Reply(200).
		BodyString(string(bytes))

	languages := map[string]float32{}
	for _, lang := range langs() {
		languages[lang] = rand.Float32()
	}

	bytes, err = json.Marshal(languages)
	require.NoError(t, err)

	gock.New("https://gitlab.com").
		Get(fmt.Sprintf("/api/v4/projects/%s/languages", prjPath)).
		Reply(200).
		BodyString(string(bytes))
}

func sha(files ...string) *string {
	return String(string(sha1.New().Sum([]byte(strings.Join(files, "-")))))
}

func String(value string) *string {
	return &value
}
