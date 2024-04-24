// Copyright 2018-2024 CERN
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// In applying this license, CERN does not waive the privileges and immunities
// granted to it by virtue of its status as an Intergovernmental Organization
// or submit itself to any jurisdiction.

package ocapi

import (
	"context"
	"net/http"

	"github.com/cs3org/reva/pkg/rhttp/global"
	"github.com/go-chi/chi/v5"
)

const roleslistMock = `{"bundles":[{"id":"2aadd357-682c-406b-8874-293091995fdd","name":"spaceadmin","type":"TYPE_ROLE","extension":"ocis-roles","displayName":"Space Admin","settings":[{"id":"b44b4054-31a2-42b8-bb71-968b15cfbd4f","name":"Drives.ReadWrite","displayName":"Manage space properties","description":"This permission allows managing space properties such as name and description.","permissionValue":{"operation":"OPERATION_READWRITE","constraint":"CONSTRAINT_ALL"},"resource":{"type":"TYPE_SYSTEM"}},{"id":"cf3faa8c-50d9-4f84-9650-ff9faf21aa9d","name":"Drives.ReadWriteEnabled","displayName":"Space ability","description":"This permission allows enabling and disabling spaces.","permissionValue":{"operation":"OPERATION_READWRITE","constraint":"CONSTRAINT_ALL"},"resource":{"type":"TYPE_SYSTEM"}},{"id":"fb60b004-c1fa-4f09-bf87-55ce7d46ac61","name":"Drives.DeleteProject","displayName":"Delete AllSpaces","description":"This permission allows to delete all spaces.","permissionValue":{"operation":"OPERATION_DELETE","constraint":"CONSTRAINT_ALL"},"resource":{"type":"TYPE_SYSTEM"}},{"id":"977f0ae6-0da2-4856-93f3-22e0a8482489","name":"Drives.ReadWriteProjectQuota","displayName":"Set Project Space Quota","description":"This permission allows managing project space quotas.","permissionValue":{"operation":"OPERATION_READWRITE","constraint":"CONSTRAINT_ALL"},"resource":{"type":"TYPE_SYSTEM"}},{"id":"79e13b30-3e22-11eb-bc51-0b9f0bad9a58","name":"Drives.Create","displayName":"Create Space","description":"This permission allows creating new spaces.","permissionValue":{"operation":"OPERATION_READWRITE","constraint":"CONSTRAINT_ALL"},"resource":{"type":"TYPE_SYSTEM"}},{"id":"016f6ddd-9501-4a0a-8ebe-64a20ee8ec82","name":"Drives.List","displayName":"List All Spaces","description":"This permission allows list all spaces.","permissionValue":{"operation":"OPERATION_READ","constraint":"CONSTRAINT_ALL"},"resource":{"type":"TYPE_SYSTEM"}},{"id":"7d81f103-0488-4853-bce5-98dcce36d649","name":"Language.ReadWrite","displayName":"Permission to read and set the language (self)","permissionValue":{"operation":"OPERATION_READWRITE","constraint":"CONSTRAINT_OWN"},"resource":{"type":"TYPE_SETTING","id":"aa8cfbe5-95d4-4f7e-a032-c3c01f5f062f"}},{"id":"ad5bb5e5-dc13-4cd3-9304-09a424564ea8","name":"EmailNotifications.ReadWriteDisabled","displayName":"Disable Email Notifications","permissionValue":{"operation":"OPERATION_READWRITE","constraint":"CONSTRAINT_OWN"},"resource":{"type":"TYPE_SETTING","id":"33ffb5d6-cd07-4dc0-afb0-84f7559ae438"}},{"id":"4e41363c-a058-40a5-aec8-958897511209","name":"AutoAcceptShares.ReadWriteDisabled","displayName":"enable/disable auto accept shares","permissionValue":{"operation":"OPERATION_READWRITE","constraint":"CONSTRAINT_OWN"},"resource":{"type":"TYPE_SETTING","id":"ec3ed4a3-3946-4efc-8f9f-76d38b12d3a9"}},{"id":"e03070e9-4362-4cc6-a872-1c7cb2eb2b8e","name":"Self.ReadWrite","displayName":"Self Management","description":"This permission gives access to self management.","permissionValue":{"operation":"OPERATION_READWRITE","constraint":"CONSTRAINT_OWN"},"resource":{"type":"TYPE_USER","id":"me"}},{"id":"79e13b30-3e22-11eb-bc51-0b9f0bad9a58","name":"Drives.Create","displayName":"Create own Space","description":"This permission allows creating a space owned by the current user.","permissionValue":{"operation":"OPERATION_CREATE","constraint":"CONSTRAINT_OWN"},"resource":{"type":"TYPE_SYSTEM"}},{"id":"11516bbd-7157-49e1-b6ac-d00c820f980b","name":"PublicLink.Write","displayName":"Write publiclink","description":"This permission permits to write a public link.","permissionValue":{"operation":"OPERATION_WRITE","constraint":"CONSTRAINT_ALL"},"resource":{"type":"TYPE_SHARE"}},{"id":"e9a697c5-c67b-40fc-982b-bcf628e9916d","name":"ReadOnlyPublicLinkPassword.Delete","displayName":"Delete Read-Only Public link password","description":"This permission permits to opt out of a public link password enforcement.","permissionValue":{"operation":"OPERATION_WRITE","constraint":"CONSTRAINT_ALL"},"resource":{"type":"TYPE_SHARE"}}],"resource":{"type":"TYPE_SYSTEM"}},{"id":"38071a68-456a-4553-846a-fa67bf5596cc","name":"user-light","type":"TYPE_ROLE","extension":"ocis-roles","displayName":"User Light","settings":[{"id":"7d81f103-0488-4853-bce5-98dcce36d649","name":"Language.ReadWrite","displayName":"Permission to read and set the language (self)","permissionValue":{"operation":"OPERATION_READWRITE","constraint":"CONSTRAINT_OWN"},"resource":{"type":"TYPE_SETTING","id":"aa8cfbe5-95d4-4f7e-a032-c3c01f5f062f"}},{"id":"ad5bb5e5-dc13-4cd3-9304-09a424564ea8","name":"EmailNotifications.ReadWriteDisabled","displayName":"Disable Email Notifications","permissionValue":{"operation":"OPERATION_READWRITE","constraint":"CONSTRAINT_OWN"},"resource":{"type":"TYPE_SETTING","id":"33ffb5d6-cd07-4dc0-afb0-84f7559ae438"}},{"id":"4e41363c-a058-40a5-aec8-958897511209","name":"AutoAcceptShares.ReadWriteDisabled","displayName":"enable/disable auto accept shares","permissionValue":{"operation":"OPERATION_READWRITE","constraint":"CONSTRAINT_OWN"},"resource":{"type":"TYPE_SETTING","id":"ec3ed4a3-3946-4efc-8f9f-76d38b12d3a9"}}],"resource":{"type":"TYPE_SYSTEM"}},{"id":"71881883-1768-46bd-a24d-a356a2afdf7f","name":"admin","type":"TYPE_ROLE","extension":"ocis-roles","displayName":"Admin","settings":[{"id":"a53e601e-571f-4f86-8fec-d4576ef49c62","name":"Roles.ReadWrite","displayName":"Role Management","description":"This permission gives full access to everything that is related to role management.","permissionValue":{"operation":"OPERATION_READWRITE","constraint":"CONSTRAINT_ALL"},"resource":{"type":"TYPE_USER","id":"all"}},{"id":"3d58f441-4a05-42f8-9411-ef5874528ae1","name":"Settings.ReadWrite","displayName":"Settings Management","description":"This permission gives full access to everything that is related to settings management.","permissionValue":{"operation":"OPERATION_READWRITE","constraint":"CONSTRAINT_ALL"},"resource":{"type":"TYPE_USER","id":"all"}},{"id":"7d81f103-0488-4853-bce5-98dcce36d649","name":"Language.ReadWrite","displayName":"Permission to read and set the language (anyone)","permissionValue":{"operation":"OPERATION_READWRITE","constraint":"CONSTRAINT_ALL"},"resource":{"type":"TYPE_SETTING","id":"aa8cfbe5-95d4-4f7e-a032-c3c01f5f062f"}},{"id":"ad5bb5e5-dc13-4cd3-9304-09a424564ea8","name":"EmailNotifications.ReadWriteDisabled","displayName":"Disable Email Notifications","permissionValue":{"operation":"OPERATION_READWRITE","constraint":"CONSTRAINT_OWN"},"resource":{"type":"TYPE_SETTING","id":"33ffb5d6-cd07-4dc0-afb0-84f7559ae438"}},{"id":"4e41363c-a058-40a5-aec8-958897511209","name":"AutoAcceptShares.ReadWriteDisabled","displayName":"enable/disable auto accept shares","permissionValue":{"operation":"OPERATION_READWRITE","constraint":"CONSTRAINT_OWN"},"resource":{"type":"TYPE_SETTING","id":"ec3ed4a3-3946-4efc-8f9f-76d38b12d3a9"}},{"id":"8e587774-d929-4215-910b-a317b1e80f73","name":"Accounts.ReadWrite","displayName":"Account Management","description":"This permission gives full access to everything that is related to account management.","permissionValue":{"operation":"OPERATION_READWRITE","constraint":"CONSTRAINT_ALL"},"resource":{"type":"TYPE_USER","id":"all"}},{"id":"522adfbe-5908-45b4-b135-41979de73245","name":"Groups.ReadWrite","displayName":"Group Management","description":"This permission gives full access to everything that is related to group management.","permissionValue":{"operation":"OPERATION_READWRITE","constraint":"CONSTRAINT_ALL"},"resource":{"type":"TYPE_GROUP","id":"all"}},{"id":"4e6f9709-f9e7-44f1-95d4-b762d27b7896","name":"Drives.ReadWritePersonalQuota","displayName":"Set Personal Space Quota","description":"This permission allows managing personal space quotas.","permissionValue":{"operation":"OPERATION_READWRITE","constraint":"CONSTRAINT_ALL"},"resource":{"type":"TYPE_SYSTEM"}},{"id":"977f0ae6-0da2-4856-93f3-22e0a8482489","name":"Drives.ReadWriteProjectQuota","displayName":"Set Project Space Quota","description":"This permission allows managing project space quotas.","permissionValue":{"operation":"OPERATION_READWRITE","constraint":"CONSTRAINT_ALL"},"resource":{"type":"TYPE_SYSTEM"}},{"id":"79e13b30-3e22-11eb-bc51-0b9f0bad9a58","name":"Drives.Create","displayName":"Create Space","description":"This permission allows creating new spaces.","permissionValue":{"operation":"OPERATION_READWRITE","constraint":"CONSTRAINT_ALL"},"resource":{"type":"TYPE_SYSTEM"}},{"id":"016f6ddd-9501-4a0a-8ebe-64a20ee8ec82","name":"Drives.List","displayName":"List All Spaces","description":"This permission allows listing all spaces.","permissionValue":{"operation":"OPERATION_READ","constraint":"CONSTRAINT_ALL"},"resource":{"type":"TYPE_SYSTEM"}},{"id":"5de9fe0a-4bc5-4a47-b758-28f370caf169","name":"Drives.DeletePersonal","displayName":"Delete All Home Spaces","description":"This permission allows deleting home spaces.","permissionValue":{"operation":"OPERATION_DELETE","constraint":"CONSTRAINT_ALL"},"resource":{"type":"TYPE_SYSTEM"}},{"id":"fb60b004-c1fa-4f09-bf87-55ce7d46ac61","name":"Drives.DeleteProject","displayName":"Delete AllSpaces","description":"This permission allows deleting all spaces.","permissionValue":{"operation":"OPERATION_DELETE","constraint":"CONSTRAINT_ALL"},"resource":{"type":"TYPE_SYSTEM"}},{"id":"ed83fc10-1f54-4a9e-b5a7-fb517f5f3e01","name":"Logo.Write","displayName":"Change logo","description":"This permission permits to change the system logo.","permissionValue":{"operation":"OPERATION_READWRITE","constraint":"CONSTRAINT_ALL"},"resource":{"type":"TYPE_SYSTEM"}},{"id":"11516bbd-7157-49e1-b6ac-d00c820f980b","name":"PublicLink.Write","displayName":"Write publiclink","description":"This permission allows creating public links.","permissionValue":{"operation":"OPERATION_WRITE","constraint":"CONSTRAINT_ALL"},"resource":{"type":"TYPE_SHARE"}},{"id":"e9a697c5-c67b-40fc-982b-bcf628e9916d","name":"ReadOnlyPublicLinkPassword.Delete","displayName":"Delete Read-Only Public link password","description":"This permission permits to opt out of a public link password enforcement.","permissionValue":{"operation":"OPERATION_WRITE","constraint":"CONSTRAINT_ALL"},"resource":{"type":"TYPE_SHARE"}},{"id":"b44b4054-31a2-42b8-bb71-968b15cfbd4f","name":"Drives.ReadWrite","displayName":"Manage space properties","description":"This permission allows managing space properties such as name and description.","permissionValue":{"operation":"OPERATION_READWRITE","constraint":"CONSTRAINT_ALL"},"resource":{"type":"TYPE_SYSTEM"}},{"id":"cf3faa8c-50d9-4f84-9650-ff9faf21aa9d","name":"Drives.ReadWriteEnabled","displayName":"Space ability","description":"This permission allows enabling and disabling spaces.","permissionValue":{"operation":"OPERATION_READWRITE","constraint":"CONSTRAINT_ALL"},"resource":{"type":"TYPE_SYSTEM"}}],"resource":{"type":"TYPE_SYSTEM"}},{"id":"d7beeea8-8ff4-406b-8fb6-ab2dd81e6b11","name":"user","type":"TYPE_ROLE","extension":"ocis-roles","displayName":"User","settings":[{"id":"7d81f103-0488-4853-bce5-98dcce36d649","name":"Language.ReadWrite","displayName":"Permission to read and set the language (self)","permissionValue":{"operation":"OPERATION_READWRITE","constraint":"CONSTRAINT_OWN"},"resource":{"type":"TYPE_SETTING","id":"aa8cfbe5-95d4-4f7e-a032-c3c01f5f062f"}},{"id":"ad5bb5e5-dc13-4cd3-9304-09a424564ea8","name":"EmailNotifications.ReadWriteDisabled","displayName":"Disable Email Notifications","permissionValue":{"operation":"OPERATION_READWRITE","constraint":"CONSTRAINT_OWN"},"resource":{"type":"TYPE_SETTING","id":"33ffb5d6-cd07-4dc0-afb0-84f7559ae438"}},{"id":"4e41363c-a058-40a5-aec8-958897511209","name":"AutoAcceptShares.ReadWriteDisabled","displayName":"enable/disable auto accept shares","permissionValue":{"operation":"OPERATION_READWRITE","constraint":"CONSTRAINT_OWN"},"resource":{"type":"TYPE_SETTING","id":"ec3ed4a3-3946-4efc-8f9f-76d38b12d3a9"}},{"id":"e03070e9-4362-4cc6-a872-1c7cb2eb2b8e","name":"Self.ReadWrite","displayName":"Self Management","description":"This permission gives access to self management.","permissionValue":{"operation":"OPERATION_READWRITE","constraint":"CONSTRAINT_OWN"},"resource":{"type":"TYPE_USER","id":"me"}},{"id":"79e13b30-3e22-11eb-bc51-0b9f0bad9a58","name":"Drives.Create","displayName":"Create own Space","description":"This permission allows creating a space owned by the current user.","permissionValue":{"operation":"OPERATION_CREATE","constraint":"CONSTRAINT_OWN"},"resource":{"type":"TYPE_SYSTEM"}},{"id":"11516bbd-7157-49e1-b6ac-d00c820f980b","name":"PublicLink.Write","displayName":"Write publiclink","description":"This permission permits to write a public link.","permissionValue":{"operation":"OPERATION_WRITE","constraint":"CONSTRAINT_ALL"},"resource":{"type":"TYPE_SHARE"}}],"resource":{"type":"TYPE_SYSTEM"}}]}`

const assigmentMock = `{"assignments":[{"id":"412cbb5a-48cf-401b-8709-6f88d1d33b9d","accountUuid":"619201e3-d9ca-41ab-a03d-c995e3f876f6","roleId":"71881883-1768-46bd-a24d-a356a2afdf7f"}]}`

// TODO(lopresti) this is currently mocked for a "primary" user, need to remove some of those permissions for other types.
const permissionsMock = `{"permissions": [
	"ReadOnlyPublicLinkPassword.Delete.all",
	"EmailNotifications.ReadWriteDisabled.own",
	"Favorites.Write.own",
	"AutoAcceptShares.ReadWriteDisabled.own",
	"PublicLink.Write.all",
	"Drives.ReadWriteEnabled.all",
	"Language.ReadWrite.all",
	"Favorites.List.own",
	"Drives.ReadWrite.all",
	"Shares.Write.all"
]}`

const valuesMock = `{"values":[{"identifier":{"extension":"ocis-accounts","bundle":"profile","setting":"language"},"value":{"bundleId":"2a506de7-99bd-4f0d-994e-c38e72c28fd9","settingId":"aa8cfbe5-95d4-4f7e-a032-c3c01f5f062f","accountUuid":"619201e3-d9ca-41ab-a03d-c995e3f876f6","resource":{"type":"TYPE_USER"},"listValue":{"values":[{"stringValue":"en"}]}}}]}`

func init() {
	global.Register("ocapi", New)
}

func New(ctx context.Context, m map[string]any) (global.Service, error) {
	r := chi.NewRouter()

	r.Post("/v0/settings/roles-list", mockResponse(roleslistMock))
	r.Post("/v0/settings/assignments-list", mockResponse(assigmentMock))
	r.Post("/v0/settings/permissions-list", mockResponse(permissionsMock))
	r.Post("/v0/settings/values-list", mockResponse(valuesMock))

	return svc{r: r}, nil
}

func mockResponse(content string) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(content))
	})
}

type svc struct {
	r *chi.Mux
}

func (s svc) Handler() http.Handler {
	return s.r
}

func (s svc) Prefix() string { return "api" }

func (s svc) Close() error { return nil }

func (s svc) Unprotected() []string { return []string{"/"} }
