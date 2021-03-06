// Copyright Amazon.com Inc or its affiliates and the project contributors
// Written by James Shubin <purple@amazon.com> and the project contributors
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may not
// use this file except in compliance with the License. You may obtain a copy of
// the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
// WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
// License for the specific language governing permissions and limitations under
// the License.
//
// We will never require a CLA to submit a patch. All contributions follow the
// `inbound == outbound` rule.
//
// This is not an official Amazon product. Amazon does not offer support for
// this project.

package lib

import (
	"fmt"
	"sort"
	"strings"

	"github.com/awslabs/yesiscan/interfaces"
	"github.com/awslabs/yesiscan/util"
	"github.com/awslabs/yesiscan/util/licenses"

	colour "github.com/fatih/color"
)

const (
	// UseColour specifies whether we use ANSI terminal colours or not.
	UseColour = true

	// DefaultProfileName is the name given to the built-in "include all"
	// profile.
	DefaultProfileName = "default"
)

// ProfileConfig is the datastructure representing the profile config that is
// used for the .json files on disk.
type ProfileConfig struct {

	// Licenses is the list of license SPDX ID's to match.
	Licenses []string `json:"licenses"`

	// Exclude these licenses from match instead of including by default.
	Exclude bool `json:"exclude"`

	// Comment adds a user friendly comment for this file.
	Comment string `json:"comment"`
}

// ProfileData is the parsed version of ProfileConfig with real license structs.
type ProfileData struct {

	// Licenses is the list of license SPDX ID's to match.
	Licenses []*licenses.License

	// Exclude these licenses from match instead of including by default.
	Exclude bool
}

// SimpleProfiles is a simple way to filter the results. This is the first
// filter function created and is mostly used for an initial POC. It is the
// more complicated successor to the SimpleResults function.
func SimpleProfiles(results interfaces.ResultSet, profile *ProfileData, backendWeights map[interfaces.Backend]float64) (string, error) {
	if len(results) == 0 {
		return "", fmt.Errorf("no results obtained")
	}

	str := ""
	// XXX: handle dir's in here specially and merge in their weights with child paths!
Loop:
	for uri, m := range results { // FIXME: sort and process properly
		bs := []*AnnotatedBackend{}
		ttl := 0.0      // total weight for the set of backends at this uri
		skipUri := true // assume we skip
		for backend, result := range m {
			if profile == nil {
				skipUri = false
			} else {
				// TODO: memoize this for performance
				count := len(licenses.Union(profile.Licenses, result.Licenses))
				// are there licenses that match in our profile?
				if count > 0 && !profile.Exclude {
					skipUri = false
				}

				// are there licenses we didn't account for?
				if len(result.Licenses) > count && profile.Exclude {
					skipUri = false
				}
			}

			weight, exists := backendWeights[backend]
			if !exists {
				return "", fmt.Errorf("no weight found for backend: %s", backend.String())
			}
			b := &AnnotatedBackend{
				Backend: backend,
				Weight:  weight,
			}
			bs = append(bs, b)
			ttl += weight
		}
		if skipUri { // we don't want to display this Uri (this file)
			continue Loop
		}
		f := 0.0 // NOTE: confidence *if* the different results agree!
		//for backend, result := range m {
		for _, b := range bs { // for backend, result := range m
			backend := b.Backend
			weight := b.Weight // backendWeights[backend]
			result := m[backend]
			scale := weight / ttl
			b.ScaledConfidence = result.Confidence * scale
			f = f + b.ScaledConfidence
		}

		sort.Sort(sort.Reverse(SortedBackends(bs)))
		display := uri // show the URI
		smartURI := util.SmartURI(uri)
		hyperlink := util.ShellHyperlinkEncode(display, smartURI)
		str += fmt.Sprintf("%s (%.2f%%)\n", hyperlink, f*100.0)
		for _, b := range bs { // for backend, result := range m
			backend := b.Backend
			weight := b.Weight // backendWeights[backend]
			result := m[backend]

			l := licenses.Join(result.Licenses)
			if UseColour && profile != nil {
				redString := colour.New(colour.FgRed).Add(colour.Bold).SprintFunc()
				ll := []string{}
				// only colour the matched ones!
				for _, x := range result.Licenses {
					r := x.String()
					inList := licenses.InList(x, profile.Licenses)
					if inList && !profile.Exclude || !inList && profile.Exclude {
						r = redString(x.String())
					}

					ll = append(ll, r)
				}
				l = strings.Join(ll, ", ")
			}

			str += fmt.Sprintf("    %s (%.2f/%.2f)  %s (%.2f%%)\n", backend.String(), weight, ttl, l, result.Confidence*100.0)
			if !debug {
				continue
			}
			it := result.Meta.Iterator // at least one must be present
			for {
				str += fmt.Sprintf("        %s\n", it)
				newIt := it.GetIterator()
				if newIt == nil {
					break
				}
				it = newIt
			}
			if parser := it.GetParser(); parser != nil {
				str += fmt.Sprintf("            %s\n", parser)
			}
		}
	}
	if str == "" {
		return "<no results>", nil // TODO: error instead?
	}
	return str, nil
}
