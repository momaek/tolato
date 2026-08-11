<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { Plus, Trash2, Loader2, Users, Server, ShieldCheck, X } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import { Separator } from '@/components/ui/separator'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from '@/components/ui/dialog'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import {
  getUsers,
  getNodes,
  getUserGroups,
  createUserGroup,
  updateUserGroup,
  deleteUserGroup,
  getNodeGroups,
  createNodeGroup,
  updateNodeGroup,
  deleteNodeGroup,
  getGrants,
  createGrant,
  deleteGrant,
} from '@/services/api'
import type {
  UserItem,
  NodeListItem,
  GroupItem,
  GrantItem,
  NodeLevel,
  SubjectType,
  ObjectType,
} from '@/types/api'

const { t } = useI18n()

const tab = ref<'grants' | 'user_groups' | 'node_groups'>('grants')
const loading = ref(true)

const users = ref<UserItem[]>([])
const nodes = ref<NodeListItem[]>([])
const userGroups = ref<GroupItem[]>([])
const nodeGroups = ref<GroupItem[]>([])
const grants = ref<GrantItem[]>([])

function reportError(err: unknown, fallback: string) {
  const message = (err as { response?: { data?: { message?: string } } })?.response?.data?.message
  toast.error(message || fallback)
}

async function loadAll() {
  loading.value = true
  try {
    const [u, n, ug, ng, g] = await Promise.all([
      getUsers(),
      getNodes(),
      getUserGroups(),
      getNodeGroups(),
      getGrants(),
    ])
    users.value = u
    nodes.value = n
    userGroups.value = ug
    nodeGroups.value = ng
    grants.value = g
  } catch (err) {
    reportError(err, t('permissions.failedToLoad'))
  } finally {
    loading.value = false
  }
}

onMounted(loadAll)

function nodeLabel(n: NodeListItem) {
  return n.alias || n.name
}

// An SSO username can be as anonymous as "user" — the display name is what
// tells you which person a picker row or a grant actually refers to.
function userLabel(u: UserItem) {
  if (!u.display_name || u.display_name === u.username) return u.username
  return `${u.display_name} (${u.username})`
}

// --- Grants ---

const showGrantDialog = ref(false)
const savingGrant = ref(false)
const grantForm = ref({
  subject_type: 'user_group' as SubjectType,
  subject_id: '',
  object_type: 'node_group' as ObjectType,
  // Several objects at once: one subject usually needs more than one group,
  // and doing that as N trips through the dialog is the same rule typed N times.
  object_ids: [] as string[],
  level: 'viewer' as NodeLevel,
})
const objectFilter = ref('')

/** Options for the chosen subject kind. */
const subjectOptions = computed(() =>
  grantForm.value.subject_type === 'user'
    ? users.value.map((u) => ({ id: u.id, label: userLabel(u) }))
    : userGroups.value.map((g) => ({ id: g.id, label: g.name })),
)

const objectOptions = computed(() =>
  grantForm.value.object_type === 'node'
    ? nodes.value.map((n) => ({ id: n.id, label: nodeLabel(n) }))
    : nodeGroups.value.map((g) => ({ id: g.id, label: g.name })),
)

/** The list as typed into the filter box — the ticks themselves survive filtering. */
const visibleObjectOptions = computed(() => {
  const q = objectFilter.value.trim().toLowerCase()
  if (!q) return objectOptions.value
  return objectOptions.value.filter((o) => o.label.toLowerCase().includes(q))
})

// Shown as chips above the list, because a tick scrolled out of view — or
// filtered out of it — is a choice the dialog is keeping but no longer showing.
const selectedObjects = computed(() =>
  objectOptions.value.filter((o) => grantForm.value.object_ids.includes(o.id)),
)

const allVisibleSelected = computed(
  () =>
    visibleObjectOptions.value.length > 0 &&
    visibleObjectOptions.value.every((o) => grantForm.value.object_ids.includes(o.id)),
)

// "all nodes" needs no object id; everything else needs at least one.
const canSaveGrant = computed(
  () =>
    !!grantForm.value.subject_id &&
    (grantForm.value.object_type === 'all' || grantForm.value.object_ids.length > 0),
)

function toggleObject(id: string) {
  const list = grantForm.value.object_ids
  const i = list.indexOf(id)
  if (i >= 0) list.splice(i, 1)
  else list.push(id)
}

// Adds what the filter is currently showing rather than the whole catalogue,
// so "filter, select all" is a way to grant a whole environment at once.
function selectAllVisible() {
  for (const o of visibleObjectOptions.value) {
    if (!grantForm.value.object_ids.includes(o.id)) grantForm.value.object_ids.push(o.id)
  }
}

function setObjectType(type: ObjectType) {
  grantForm.value.object_type = type
  grantForm.value.object_ids = []
  objectFilter.value = ''
}

function openGrantDialog() {
  grantForm.value = {
    subject_type: 'user_group',
    subject_id: '',
    object_type: 'node_group',
    object_ids: [],
    level: 'viewer',
  }
  objectFilter.value = ''
  showGrantDialog.value = true
}

async function handleCreateGrant() {
  if (!canSaveGrant.value) return
  savingGrant.value = true
  try {
    await createGrant({
      subject_type: grantForm.value.subject_type,
      subject_id: grantForm.value.subject_id,
      object_type: grantForm.value.object_type,
      object_ids: grantForm.value.object_type === 'all' ? undefined : grantForm.value.object_ids,
      level: grantForm.value.level,
    })
    // One click can now write several rules, so say how many landed — otherwise
    // the only feedback is a table the reader has to re-count.
    const count = grantForm.value.object_type === 'all' ? 1 : grantForm.value.object_ids.length
    showGrantDialog.value = false
    await loadAll()
    toast.success(t('permissions.grantsSaved', { count }))
  } catch (err) {
    reportError(err, t('permissions.failedToSave'))
  } finally {
    savingGrant.value = false
  }
}

// Grouped by subject rather than newest-first: a subject now holds several
// rules at once, and the question being asked of this table is "what can this
// person reach", which reads badly when their rows are scattered by age.
const groupedGrants = computed(() => {
  const rows = [...grants.value].sort(
    (a, b) =>
      a.subject_name.localeCompare(b.subject_name) || a.object_name.localeCompare(b.object_name),
  )
  return rows.map((g, i) => ({
    ...g,
    firstOfSubject: i === 0 || rows[i - 1].subject_id !== g.subject_id,
  }))
})

async function handleDeleteGrant(id: string) {
  try {
    await deleteGrant(id)
    await loadAll()
  } catch (err) {
    reportError(err, t('permissions.failedToDelete'))
  }
}

// --- Groups (both kinds share one editor) ---

const showGroupDialog = ref(false)
const savingGroup = ref(false)
const groupKind = ref<'user' | 'node'>('user')
const editingGroup = ref<GroupItem | null>(null)
const groupForm = ref({ name: '', description: '', member_ids: [] as string[] })

const groupMemberOptions = computed(() =>
  groupKind.value === 'user'
    ? users.value.map((u) => ({ id: u.id, label: userLabel(u) }))
    : nodes.value.map((n) => ({ id: n.id, label: nodeLabel(n) })),
)

function openGroupDialog(kind: 'user' | 'node', group: GroupItem | null) {
  groupKind.value = kind
  editingGroup.value = group
  groupForm.value = group
    ? { name: group.name, description: group.description ?? '', member_ids: [...group.member_ids] }
    : { name: '', description: '', member_ids: [] }
  showGroupDialog.value = true
}

function toggleMember(id: string) {
  const list = groupForm.value.member_ids
  const i = list.indexOf(id)
  if (i >= 0) list.splice(i, 1)
  else list.push(id)
}

async function handleSaveGroup() {
  if (!groupForm.value.name.trim()) return
  savingGroup.value = true
  const payload = {
    name: groupForm.value.name.trim(),
    description: groupForm.value.description.trim(),
    member_ids: groupForm.value.member_ids,
  }
  try {
    if (editingGroup.value) {
      await (groupKind.value === 'user' ? updateUserGroup : updateNodeGroup)(
        editingGroup.value.id,
        payload,
      )
    } else {
      await (groupKind.value === 'user' ? createUserGroup : createNodeGroup)(payload)
    }
    showGroupDialog.value = false
    await loadAll()
  } catch (err) {
    reportError(err, t('permissions.failedToSave'))
  } finally {
    savingGroup.value = false
  }
}

const deleteTarget = ref<{ kind: 'user' | 'node'; group: GroupItem } | null>(null)

async function handleDeleteGroup() {
  if (!deleteTarget.value) return
  const { kind, group } = deleteTarget.value
  try {
    await (kind === 'user' ? deleteUserGroup : deleteNodeGroup)(group.id)
    deleteTarget.value = null
    await loadAll()
  } catch (err) {
    reportError(err, t('permissions.failedToDelete'))
  }
}

const levelLabels: Record<NodeLevel, string> = {
  viewer: 'permissions.levelViewer',
  operator: 'permissions.levelOperator',
  manager: 'permissions.levelManager',
}
</script>

<template>
  <!-- The layout's <main> is overflow-hidden, so this page has to own its
       scrolling — without it a long grant list is simply unreachable. -->
  <div class="h-full overflow-y-auto p-6">
    <div>
      <h1 class="text-lg font-semibold">{{ $t('permissions.title') }}</h1>
      <p class="mt-1 text-sm" style="color: var(--muted-foreground)">
        {{ $t('permissions.description') }}
      </p>
    </div>

    <div class="mt-4 flex gap-2">
      <Button
        v-for="item in [
          { id: 'grants', label: $t('permissions.tabGrants'), icon: ShieldCheck },
          { id: 'user_groups', label: $t('permissions.tabUserGroups'), icon: Users },
          { id: 'node_groups', label: $t('permissions.tabNodeGroups'), icon: Server },
        ]"
        :key="item.id"
        :variant="tab === item.id ? 'default' : 'outline'"
        size="sm"
        @click="tab = item.id as typeof tab"
      >
        <component :is="item.icon" class="mr-2 h-3.5 w-3.5" />
        {{ item.label }}
      </Button>
    </div>

    <Separator class="my-6" />

    <div v-if="loading" class="flex justify-center py-12">
      <Loader2 class="h-5 w-5 animate-spin" style="color: var(--muted-foreground)" />
    </div>

    <!-- Grants -->
    <template v-else-if="tab === 'grants'">
      <div class="mb-4 flex items-center justify-between">
        <p class="text-sm" style="color: var(--muted-foreground)">
          {{ $t('permissions.grantsHelp') }}
        </p>
        <Button @click="openGrantDialog">
          <Plus class="mr-2 h-4 w-4" />
          {{ $t('permissions.addGrant') }}
        </Button>
      </div>

      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{{ $t('permissions.who') }}</TableHead>
            <TableHead>{{ $t('permissions.what') }}</TableHead>
            <TableHead>{{ $t('permissions.level') }}</TableHead>
            <TableHead>{{ $t('common.actions') }}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          <TableRow v-for="g in groupedGrants" :key="g.id">
            <TableCell :class="g.firstOfSubject ? '' : 'py-1'">
              <template v-if="g.firstOfSubject">
                <span class="font-medium">{{ g.subject_name }}</span>
                <Badge variant="outline" class="ml-2 text-[10px]">
                  {{ g.subject_type === 'user' ? $t('permissions.kindUser') : $t('permissions.kindUserGroup') }}
                </Badge>
              </template>
            </TableCell>
            <TableCell>
              <span class="font-medium">
                {{ g.object_type === 'all' ? $t('permissions.allNodes') : g.object_name }}
              </span>
              <Badge v-if="g.object_type !== 'all'" variant="outline" class="ml-2 text-[10px]">
                {{ g.object_type === 'node' ? $t('permissions.kindNode') : $t('permissions.kindNodeGroup') }}
              </Badge>
            </TableCell>
            <TableCell><Badge variant="secondary">{{ $t(levelLabels[g.level]) }}</Badge></TableCell>
            <TableCell>
              <Button size="icon-sm" variant="ghost" @click="handleDeleteGrant(g.id)">
                <Trash2 class="h-3.5 w-3.5" style="color: var(--color-error-foreground)" />
              </Button>
            </TableCell>
          </TableRow>
          <TableRow v-if="grants.length === 0">
            <TableCell :colspan="4" class="py-8 text-center text-sm" style="color: var(--muted-foreground)">
              {{ $t('permissions.noGrants') }}
            </TableCell>
          </TableRow>
        </TableBody>
      </Table>
    </template>

    <!-- Groups (user or node) -->
    <template v-else>
      <div class="mb-4 flex items-center justify-between">
        <p class="text-sm" style="color: var(--muted-foreground)">
          {{ tab === 'user_groups' ? $t('permissions.userGroupsHelp') : $t('permissions.nodeGroupsHelp') }}
        </p>
        <Button @click="openGroupDialog(tab === 'user_groups' ? 'user' : 'node', null)">
          <Plus class="mr-2 h-4 w-4" />
          {{ $t('permissions.addGroup') }}
        </Button>
      </div>

      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{{ $t('common.name') }}</TableHead>
            <TableHead>{{ $t('permissions.groupDescription') }}</TableHead>
            <TableHead>{{ $t('permissions.members') }}</TableHead>
            <TableHead>{{ $t('common.actions') }}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          <TableRow v-for="g in tab === 'user_groups' ? userGroups : nodeGroups" :key="g.id">
            <TableCell class="font-medium">{{ g.name }}</TableCell>
            <TableCell class="text-sm" style="color: var(--muted-foreground)">
              {{ g.description || '—' }}
            </TableCell>
            <TableCell>{{ g.member_ids.length }}</TableCell>
            <TableCell>
              <div class="flex gap-1">
                <Button
                  size="sm"
                  variant="ghost"
                  @click="openGroupDialog(tab === 'user_groups' ? 'user' : 'node', g)"
                >
                  {{ $t('permissions.edit') }}
                </Button>
                <Button
                  size="icon-sm"
                  variant="ghost"
                  @click="deleteTarget = { kind: tab === 'user_groups' ? 'user' : 'node', group: g }"
                >
                  <Trash2 class="h-3.5 w-3.5" style="color: var(--color-error-foreground)" />
                </Button>
              </div>
            </TableCell>
          </TableRow>
          <TableRow v-if="(tab === 'user_groups' ? userGroups : nodeGroups).length === 0">
            <TableCell :colspan="4" class="py-8 text-center text-sm" style="color: var(--muted-foreground)">
              {{ $t('permissions.noGroups') }}
            </TableCell>
          </TableRow>
        </TableBody>
      </Table>
    </template>

    <!-- Grant dialog -->
    <Dialog :open="showGrantDialog" @update:open="showGrantDialog = $event">
      <DialogContent class="max-w-lg">
        <DialogHeader>
          <DialogTitle>{{ $t('permissions.addGrant') }}</DialogTitle>
        </DialogHeader>
        <!-- The dialog is fixed and centred, so without a scroll cap here a long
             selection grows it past both edges of the screen. -->
        <div class="max-h-[65vh] space-y-4 overflow-y-auto px-1">
          <div class="grid grid-cols-2 gap-3">
            <div>
              <label class="text-sm font-medium">{{ $t('permissions.who') }}</label>
              <Select
                :model-value="grantForm.subject_type"
                @update:model-value="(v) => { grantForm.subject_type = v as SubjectType; grantForm.subject_id = '' }"
              >
                <SelectTrigger class="mt-1.5"><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="user_group">{{ $t('permissions.kindUserGroup') }}</SelectItem>
                  <SelectItem value="user">{{ $t('permissions.kindUser') }}</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div>
              <label class="text-sm font-medium">&nbsp;</label>
              <Select v-model="grantForm.subject_id">
                <SelectTrigger class="mt-1.5">
                  <SelectValue :placeholder="$t('permissions.choose')" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem v-for="o in subjectOptions" :key="o.id" :value="o.id">
                    {{ o.label }}
                  </SelectItem>
                </SelectContent>
              </Select>
            </div>
          </div>

          <div>
            <div class="flex items-center justify-between">
              <label class="text-sm font-medium">{{ $t('permissions.what') }}</label>
              <div v-if="grantForm.object_type !== 'all'" class="flex gap-3 text-xs">
                <button
                  v-if="!allVisibleSelected && visibleObjectOptions.length > 1"
                  class="hover:underline"
                  style="color: var(--muted-foreground)"
                  @click="selectAllVisible"
                >
                  {{ $t('permissions.selectAll') }}
                </button>
                <button
                  v-if="grantForm.object_ids.length > 0"
                  class="hover:underline"
                  style="color: var(--muted-foreground)"
                  @click="grantForm.object_ids = []"
                >
                  {{ $t('permissions.clearSelection') }}
                </button>
              </div>
            </div>
            <Select
              :model-value="grantForm.object_type"
              @update:model-value="(v) => setObjectType(v as ObjectType)"
            >
              <SelectTrigger class="mt-1.5"><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectItem value="node_group">{{ $t('permissions.kindNodeGroup') }}</SelectItem>
                <SelectItem value="node">{{ $t('permissions.kindNode') }}</SelectItem>
                <SelectItem value="all">{{ $t('permissions.allNodes') }}</SelectItem>
              </SelectContent>
            </Select>

            <template v-if="grantForm.object_type !== 'all'">
              <div
                v-if="selectedObjects.length > 0"
                class="mt-2 flex max-h-24 flex-wrap gap-1.5 overflow-y-auto"
              >
                <button
                  v-for="o in selectedObjects"
                  :key="o.id"
                  class="flex items-center gap-1 rounded-full px-2 py-0.5 text-xs"
                  style="background: var(--secondary); color: var(--secondary-foreground)"
                  @click="toggleObject(o.id)"
                >
                  {{ o.label }}
                  <X class="h-3 w-3" />
                </button>
              </div>
              <Input
                v-if="objectOptions.length > 8"
                v-model="objectFilter"
                class="mt-2"
                autocomplete="off"
                :placeholder="$t('permissions.filterPlaceholder')"
              />
              <div
                class="mt-2 max-h-44 space-y-1 overflow-y-auto rounded-lg border p-2"
                style="border-color: var(--border)"
              >
                <label
                  v-for="o in visibleObjectOptions"
                  :key="o.id"
                  class="flex cursor-pointer items-center gap-2 rounded px-2 py-1 text-sm hover:bg-[var(--secondary)]"
                >
                  <input
                    type="checkbox"
                    :checked="grantForm.object_ids.includes(o.id)"
                    @change="toggleObject(o.id)"
                  />
                  {{ o.label }}
                </label>
                <p
                  v-if="visibleObjectOptions.length === 0"
                  class="px-2 py-3 text-center text-xs"
                  style="color: var(--muted-foreground)"
                >
                  {{ $t('permissions.noCandidates') }}
                </p>
              </div>
              <p class="mt-1.5 text-xs" style="color: var(--muted-foreground)">
                {{ $t('permissions.selectedCount', { count: grantForm.object_ids.length }) }}
              </p>
            </template>
          </div>

          <div>
            <label class="text-sm font-medium">{{ $t('permissions.level') }}</label>
            <Select v-model="grantForm.level">
              <SelectTrigger class="mt-1.5"><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectItem value="viewer">{{ $t('permissions.levelViewer') }}</SelectItem>
                <SelectItem value="operator">{{ $t('permissions.levelOperator') }}</SelectItem>
                <SelectItem value="manager">{{ $t('permissions.levelManager') }}</SelectItem>
              </SelectContent>
            </Select>
            <p class="mt-1.5 text-xs" style="color: var(--muted-foreground)">
              {{ $t('permissions.levelHelp') }}
            </p>
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" @click="showGrantDialog = false">{{ $t('common.cancel') }}</Button>
          <Button :disabled="!canSaveGrant || savingGrant" @click="handleCreateGrant">
            <Loader2 v-if="savingGrant" class="mr-2 h-4 w-4 animate-spin" />
            {{ $t('common.save') }}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>

    <!-- Group editor -->
    <Dialog :open="showGroupDialog" @update:open="showGroupDialog = $event">
      <DialogContent class="max-w-lg">
        <DialogHeader>
          <DialogTitle>
            {{ editingGroup ? $t('permissions.editGroup') : $t('permissions.addGroup') }}
          </DialogTitle>
        </DialogHeader>
        <div class="space-y-4">
          <div>
            <label class="text-sm font-medium">{{ $t('common.name') }}</label>
            <Input v-model="groupForm.name" class="mt-1.5" autocomplete="off" />
          </div>
          <div>
            <label class="text-sm font-medium">{{ $t('permissions.groupDescription') }}</label>
            <Input v-model="groupForm.description" class="mt-1.5" autocomplete="off" />
          </div>
          <div>
            <label class="text-sm font-medium">{{ $t('permissions.members') }}</label>
            <div
              class="mt-1.5 max-h-52 space-y-1 overflow-y-auto rounded-lg border p-2"
              style="border-color: var(--border)"
            >
              <label
                v-for="o in groupMemberOptions"
                :key="o.id"
                class="flex cursor-pointer items-center gap-2 rounded px-2 py-1 text-sm hover:bg-[var(--secondary)]"
              >
                <input
                  type="checkbox"
                  :checked="groupForm.member_ids.includes(o.id)"
                  @change="toggleMember(o.id)"
                />
                {{ o.label }}
              </label>
              <p
                v-if="groupMemberOptions.length === 0"
                class="px-2 py-3 text-center text-xs"
                style="color: var(--muted-foreground)"
              >
                {{ $t('permissions.noCandidates') }}
              </p>
            </div>
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" @click="showGroupDialog = false">{{ $t('common.cancel') }}</Button>
          <Button :disabled="!groupForm.name.trim() || savingGroup" @click="handleSaveGroup">
            <Loader2 v-if="savingGroup" class="mr-2 h-4 w-4 animate-spin" />
            {{ $t('common.save') }}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>

    <!-- Group delete confirmation -->
    <Dialog :open="!!deleteTarget" @update:open="(v) => { if (!v) deleteTarget = null }">
      <DialogContent class="max-w-md">
        <DialogHeader>
          <DialogTitle>{{ $t('permissions.deleteGroup') }}</DialogTitle>
        </DialogHeader>
        <p class="text-sm" style="color: var(--muted-foreground)">
          {{ $t('permissions.deleteGroupWarning', { name: deleteTarget?.group.name }) }}
        </p>
        <DialogFooter>
          <Button variant="outline" @click="deleteTarget = null">{{ $t('common.cancel') }}</Button>
          <Button variant="destructive" @click="handleDeleteGroup">{{ $t('common.delete') }}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  </div>
</template>
