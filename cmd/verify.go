package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"net/http"

	"github.com/rmohr/bazeldnf/pkg/bazel"
	"github.com/rmohr/bazeldnf/pkg/repo"
	"github.com/sassoftware/go-rpmutils"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/openpgp"
)

type VerifyOpts struct {
	repoFile  string
	workspace string
}

var verifyopts = VerifyOpts{}

func NewVerifyCmd() *cobra.Command {

	verifyCmd := &cobra.Command{
		Use:   "verify",
		Short: "verify RPMs against gpg keys defined in repo.yaml",
		Long:  `verify RPMs against gpg keys defined in repo.yaml`,
		RunE: func(cmd *cobra.Command, args []string) error {
			repos, err := repo.LoadRepoFile(verifyopts.repoFile)
			if err != nil {
				return err
			}
			keyring := openpgp.EntityList{}
			for _, repo := range repos.Repositories {
				if !repo.Disabled && repo.GPGKey != "" {
					resp, err := http.Get(repo.GPGKey)
					if err != nil {
						return fmt.Errorf("could not fetch gpgkey %s: %v", repo.GPGKey, err)
					}
					defer resp.Body.Close()
					keys, err := openpgp.ReadArmoredKeyRing(resp.Body)
					if err != nil {
						return fmt.Errorf("could not load gpgkey %s: %v", repo.GPGKey, err)
					}
					for _, k := range keys {
						keyring = append(keyring, k)
					}
				}
			}

			workspace, err := bazel.LoadWorkspace(verifyopts.workspace)
			if err != nil {
				return fmt.Errorf("failed to open workspace %s: %v", verifyopts.workspace, err)
			}
			for _, rpm := range bazel.GetRPMs(workspace) {
				err := verify(rpm, keyring)
				if err != nil {
					return fmt.Errorf("Could not verify %s: %v", rpm.Name(), err)
				}
			}
			return nil
		},
	}

	verifyCmd.Flags().StringVarP(&verifyopts.repoFile, "repofile", "r", "repo.yaml", "repository file")
	verifyCmd.PersistentFlags().StringVarP(&verifyopts.workspace, "workspace", "w", "WORKSPACE", "Bazel workspace file")
	return verifyCmd
}

func verify(rpm *bazel.RPMRule, keyring openpgp.EntityList) (err error) {
	// Force a test. If `nil` the verification library just does no GPG check
	if keyring == nil {
		keyring = openpgp.EntityList{}
	}

	for _, url := range rpm.URLs() {
		log.Infof("Verifying %s", rpm.Name())
		sha := sha256.New()
		resp, err := http.Get(url)
		if err != nil {
			log.Infof("Faild to download %s, continuing with next url: %v ", rpm.Name(), err)
			continue
		}
		defer resp.Body.Close()
		body := io.TeeReader(resp.Body, sha)
		_, _, err = rpmutils.Verify(body, keyring)
		if err != nil {
			return err
		}
		if rpm.SHA256() != toHex(sha) {
			return fmt.Errorf("Expected sha256 sum %s, but got %s", rpm.SHA256(), toHex(sha))
		}
		break
	}
	return nil
}

func toHex(hasher hash.Hash) string {
	return hex.EncodeToString(hasher.Sum(nil))
}
