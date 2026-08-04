<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { UserPlus, Trash2, KeyRound, Loader2 } from 'lucide-vue-next'
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
import { getUsers, createUser, updateUser, deleteUser } from '@/services/api'
import { useAppStore } from '@/stores/app'
import type { UserItem, UserRole } from '@/types/api'

const { t } = useI18n()
const appStore = useAppStore()

const users = ref<UserItem[]>([])
const loading = ref(true)

/** Surfaces the server's own message — it explains refusals like "last active
 * administrator" that the UI has no way to predict. */
function reportError(err: unknown, fallback: string) {
  const message = (err as { response?: { data?: { message?: string } } })?.response?.data?.message
  toast.error(message || fallback)
}

async function load() {
  loading.value = true
  try {
    users.value = await getUsers()
  } catch (err) {
    reportError(err, t('users.failedToLoad'))
  } finally {
    loading.value = false
  }
}

onMounted(load)

// --- Create ---

const showCreateDialog = ref(false)
const creating = ref(false)
const form = ref({ username: '', password: '', display_name: '', role: 'member' as UserRole })

function openCreate() {
  form.value = { username: '', password: '', display_name: '', role: 'member' }
  showCreateDialog.value = true
}

const canSubmitCreate = computed(
  () => form.value.username.trim().length > 0 && form.value.password.length >= 8,
)

async function handleCreate() {
  if (!canSubmitCreate.value) return
  creating.value = true
  try {
    await createUser({
      username: form.value.username.trim(),
      password: form.value.password,
      display_name: form.value.display_name.trim() || undefined,
      role: form.value.role,
    })
    showCreateDialog.value = false
    toast.success(t('users.created'))
    await load()
  } catch (err) {
    reportError(err, t('users.failedToCreate'))
  } finally {
    creating.value = false
  }
}

// --- Inline edits ---

async function changeRole(user: UserItem, role: UserRole) {
  if (user.role === role) return
  try {
    await updateUser(user.id, { role })
    await load()
  } catch (err) {
    reportError(err, t('users.failedToUpdate'))
    await load() // put the Select back in sync with the server
  }
}

async function toggleStatus(user: UserItem) {
  const status = user.status === 'active' ? 'disabled' : 'active'
  try {
    await updateUser(user.id, { status })
    await load()
  } catch (err) {
    reportError(err, t('users.failedToUpdate'))
  }
}

// --- Password reset ---

const resetTarget = ref<UserItem | null>(null)
const resetPassword = ref('')
const resetting = ref(false)

function openReset(user: UserItem) {
  resetTarget.value = user
  resetPassword.value = ''
}

async function handleReset() {
  if (!resetTarget.value || resetPassword.value.length < 8) return
  resetting.value = true
  try {
    await updateUser(resetTarget.value.id, { password: resetPassword.value })
    toast.success(t('users.passwordReset'))
    resetTarget.value = null
  } catch (err) {
    reportError(err, t('users.failedToUpdate'))
  } finally {
    resetting.value = false
  }
}

// --- Delete ---

const deleteTarget = ref<UserItem | null>(null)
const deleting = ref(false)

async function handleDelete() {
  if (!deleteTarget.value) return
  deleting.value = true
  try {
    await deleteUser(deleteTarget.value.id)
    toast.success(t('users.deleted'))
    deleteTarget.value = null
    await load()
  } catch (err) {
    reportError(err, t('users.failedToDelete'))
  } finally {
    deleting.value = false
  }
}

function isSelf(user: UserItem) {
  return user.id === appStore.user?.id
}

function formatDate(value?: string) {
  return value ? new Date(value).toLocaleString() : t('common.never')
}
</script>

<template>
  <div class="p-6">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-lg font-semibold">{{ $t('users.title') }}</h1>
        <p class="mt-1 text-sm" style="color: var(--muted-foreground)">
          {{ $t('users.description') }}
        </p>
      </div>
      <Button @click="openCreate">
        <UserPlus class="mr-2 h-4 w-4" />
        {{ $t('users.addUser') }}
      </Button>
    </div>

    <Separator class="my-6" />

    <div v-if="loading" class="flex justify-center py-12">
      <Loader2 class="h-5 w-5 animate-spin" style="color: var(--muted-foreground)" />
    </div>

    <Table v-else>
      <TableHeader>
        <TableRow>
          <TableHead>{{ $t('users.username') }}</TableHead>
          <TableHead>{{ $t('users.displayName') }}</TableHead>
          <TableHead>{{ $t('users.role') }}</TableHead>
          <TableHead>{{ $t('common.status') }}</TableHead>
          <TableHead>{{ $t('users.lastLogin') }}</TableHead>
          <TableHead>{{ $t('common.actions') }}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        <TableRow v-for="user in users" :key="user.id">
          <TableCell class="font-medium">
            {{ user.username }}
            <Badge v-if="isSelf(user)" variant="secondary" class="ml-2">{{ $t('users.you') }}</Badge>
            <Badge v-if="user.auth_source === 'oidc'" variant="outline" class="ml-2">SSO</Badge>
          </TableCell>
          <TableCell>{{ user.display_name }}</TableCell>
          <TableCell>
            <Select
              :model-value="user.role"
              @update:model-value="(v) => changeRole(user, v as UserRole)"
            >
              <SelectTrigger class="h-8 w-32">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="admin">{{ $t('users.roleAdmin') }}</SelectItem>
                <SelectItem value="member">{{ $t('users.roleMember') }}</SelectItem>
              </SelectContent>
            </Select>
          </TableCell>
          <TableCell>
            <Badge :variant="user.status === 'active' ? 'default' : 'secondary'">
              {{ user.status === 'active' ? $t('common.active') : $t('users.disabled') }}
            </Badge>
          </TableCell>
          <TableCell class="text-xs" style="color: var(--muted-foreground)">
            {{ formatDate(user.last_login_at) }}
          </TableCell>
          <TableCell>
            <div class="flex items-center gap-1">
              <Button
                size="sm"
                variant="ghost"
                :disabled="isSelf(user)"
                @click="toggleStatus(user)"
              >
                {{ user.status === 'active' ? $t('users.disable') : $t('users.enable') }}
              </Button>
              <Button
                v-if="user.auth_source === 'local'"
                size="icon-sm"
                variant="ghost"
                :title="$t('users.resetPassword')"
                @click="openReset(user)"
              >
                <KeyRound class="h-3.5 w-3.5" />
              </Button>
              <Button
                size="icon-sm"
                variant="ghost"
                :disabled="isSelf(user)"
                :title="$t('common.delete')"
                @click="deleteTarget = user"
              >
                <Trash2 class="h-3.5 w-3.5" style="color: var(--color-error-foreground)" />
              </Button>
            </div>
          </TableCell>
        </TableRow>
      </TableBody>
    </Table>

    <!-- Create user -->
    <Dialog :open="showCreateDialog" @update:open="showCreateDialog = $event">
      <DialogContent class="max-w-md">
        <DialogHeader>
          <DialogTitle>{{ $t('users.addUser') }}</DialogTitle>
        </DialogHeader>
        <div class="space-y-4">
          <div>
            <label class="text-sm font-medium">{{ $t('users.username') }}</label>
            <Input v-model="form.username" class="mt-1.5" autocomplete="off" />
          </div>
          <div>
            <label class="text-sm font-medium">{{ $t('users.displayName') }}</label>
            <Input v-model="form.display_name" class="mt-1.5" autocomplete="off" />
          </div>
          <div>
            <label class="text-sm font-medium">{{ $t('users.password') }}</label>
            <Input v-model="form.password" type="password" class="mt-1.5" autocomplete="new-password" />
            <p class="mt-1 text-xs" style="color: var(--muted-foreground)">
              {{ $t('users.passwordHint') }}
            </p>
          </div>
          <div>
            <label class="text-sm font-medium">{{ $t('users.role') }}</label>
            <Select v-model="form.role">
              <SelectTrigger class="mt-1.5">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="member">{{ $t('users.roleMember') }}</SelectItem>
                <SelectItem value="admin">{{ $t('users.roleAdmin') }}</SelectItem>
              </SelectContent>
            </Select>
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" @click="showCreateDialog = false">{{ $t('common.cancel') }}</Button>
          <Button :disabled="!canSubmitCreate || creating" @click="handleCreate">
            <Loader2 v-if="creating" class="mr-2 h-4 w-4 animate-spin" />
            {{ $t('common.create') }}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>

    <!-- Reset password -->
    <Dialog :open="!!resetTarget" @update:open="(v) => { if (!v) resetTarget = null }">
      <DialogContent class="max-w-md">
        <DialogHeader>
          <DialogTitle>{{ $t('users.resetPasswordFor', { name: resetTarget?.username }) }}</DialogTitle>
        </DialogHeader>
        <div>
          <label class="text-sm font-medium">{{ $t('users.newPassword') }}</label>
          <Input v-model="resetPassword" type="password" class="mt-1.5" autocomplete="new-password" />
          <p class="mt-1 text-xs" style="color: var(--muted-foreground)">
            {{ $t('users.passwordHint') }}
          </p>
        </div>
        <DialogFooter>
          <Button variant="outline" @click="resetTarget = null">{{ $t('common.cancel') }}</Button>
          <Button :disabled="resetPassword.length < 8 || resetting" @click="handleReset">
            <Loader2 v-if="resetting" class="mr-2 h-4 w-4 animate-spin" />
            {{ $t('users.resetPassword') }}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>

    <!-- Delete confirmation -->
    <Dialog :open="!!deleteTarget" @update:open="(v) => { if (!v) deleteTarget = null }">
      <DialogContent class="max-w-md">
        <DialogHeader>
          <DialogTitle>{{ $t('users.deleteUser') }}</DialogTitle>
        </DialogHeader>
        <p class="text-sm" style="color: var(--muted-foreground)">
          {{ $t('users.deleteWarning', { name: deleteTarget?.username }) }}
        </p>
        <DialogFooter>
          <Button variant="outline" @click="deleteTarget = null">{{ $t('common.cancel') }}</Button>
          <Button variant="destructive" :disabled="deleting" @click="handleDelete">
            <Loader2 v-if="deleting" class="mr-2 h-4 w-4 animate-spin" />
            {{ $t('common.delete') }}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  </div>
</template>
