<script setup lang="ts">
// Two-factor authentication: enrol an authenticator app, or turn it
// off. The server hands out a secret that is NOT yet active, and only a
// code proving the app holds it turns it on, so an abandoned setup
// leaves the account as it was.
import { computed, onMounted, ref, watch } from 'vue'
import {
  recoveryCodesRegenerate,
  recoveryCodesStatus,
  totpDisable,
  totpEnable,
  totpSetup,
} from '@/api/auth'
import { apiErrorMessage } from '@/api/client'
import CopyButton from '@/components/CopyButton.vue'
import FormField from '@/components/FormField.vue'
import RecoveryCodes from '@/components/account/RecoveryCodes.vue'
import { useConfirm } from '@/composables/useConfirm'
import { useAuthStore } from '@/stores/auth'
import { useNotificationStore } from '@/stores/notification'

const auth = useAuthStore()
const toast = useNotificationStore()
const { confirm } = useConfirm()

const on = computed(() => auth.user?.totp_enabled === true)
const pending = ref<{ secret: string; qr: string } | null>(null)
const setupPassword = ref('')
const code = ref('')
const busy = ref(false)
const remaining = ref<number | null>(null)
const shownCodes = ref<string[] | null>(null)
const regenerating = ref(false)
const password = ref('')

async function loadRemaining() {
  if (!on.value) {
    remaining.value = null
    return
  }
  try {
    remaining.value = await recoveryCodesStatus()
  } catch (err) {
    toast.error(apiErrorMessage(err))
  }
}

onMounted(loadRemaining)
watch(on, loadRemaining)

async function start() {
  busy.value = true
  try {
    const res = await totpSetup(setupPassword.value)
    setupPassword.value = ''
    // Drawn in the browser: the otpauth URL carries the secret. The
    // library is loaded here only, an enrolment is rare.
    const { default: QRCode } = await import('qrcode')
    pending.value = {
      secret: res.secret,
      qr: await QRCode.toDataURL(res.otpauth_url, { width: 220, margin: 1 }),
    }
    code.value = ''
  } catch (err) {
    toast.error(apiErrorMessage(err))
  } finally {
    busy.value = false
  }
}

async function enable() {
  busy.value = true
  try {
    const res = await totpEnable(code.value.trim())
    pending.value = null
    code.value = ''
    toast.success('Two-factor authentication is on')
    shownCodes.value = res.recovery_codes ?? null
    await auth.refresh()
  } catch (err) {
    toast.error(apiErrorMessage(err))
  } finally {
    busy.value = false
  }
}

async function disable() {
  const ok = await confirm({
    title: 'Turn off two-factor authentication',
    message: 'Signing in will need only your password after this. Continue?',
    confirmText: 'Turn it off',
    variant: 'danger',
  })
  if (!ok) return
  busy.value = true
  try {
    await totpDisable(code.value.trim())
    code.value = ''
    toast.success('Two-factor authentication is off')
    await auth.refresh()
  } catch (err) {
    toast.error(apiErrorMessage(err))
  } finally {
    busy.value = false
  }
}

async function regenerate() {
  busy.value = true
  try {
    shownCodes.value = await recoveryCodesRegenerate(password.value)
    password.value = ''
    regenerating.value = false
    await loadRemaining()
  } catch (err) {
    toast.error(apiErrorMessage(err))
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="card">
    <div class="card-header">
      <h2>Two-factor authentication</h2>
      <span class="badge" :class="on ? 'badge-success' : 'badge-neutral'">
        {{ on ? 'on' : 'off' }}
      </span>
    </div>
    <div class="card-body">
      <template v-if="on">
        <p class="text-sm text-muted mb-3">
          Signing in asks for a code from your authenticator app. To turn that off, enter a current
          code.
        </p>
        <div class="recovery">
          <p class="text-sm text-muted mb-2">
            Recovery codes sign you in when the authenticator is gone.
            <template v-if="remaining !== null">
              <strong>{{ remaining }}</strong> of 10 left.
            </template>
          </p>
          <button
            v-if="!regenerating"
            class="btn btn-secondary btn-sm"
            :disabled="busy"
            @click="regenerating = true"
          >
            Generate new codes
          </button>
          <form v-else class="row" @submit.prevent="regenerate">
            <FormField label="Your password" for="recovery-password">
              <input
                id="recovery-password"
                v-model="password"
                type="password"
                class="form-input"
                autocomplete="current-password"
                required
              />
            </FormField>
            <button class="btn btn-primary" type="submit" :disabled="busy || !password">
              Generate
            </button>
            <button class="btn btn-secondary" type="button" @click="regenerating = false">
              Cancel
            </button>
          </form>
        </div>
        <form class="row" @submit.prevent="disable">
          <FormField label="Code from your app" for="totp-off">
            <input
              id="totp-off"
              v-model="code"
              class="form-input"
              inputmode="numeric"
              maxlength="6"
              autocomplete="one-time-code"
              placeholder="123456"
              required
            />
          </FormField>
          <button class="btn btn-danger" type="submit" :disabled="busy || !code">Turn off</button>
        </form>
      </template>

      <template v-else-if="!pending">
        <p class="text-sm text-muted mb-3">
          A second step at sign-in: a six digit code from an authenticator app on your phone.
          Confirm your password to begin.
        </p>
        <form class="row" @submit.prevent="start">
          <FormField label="Your password" for="totp-setup-password">
            <input
              id="totp-setup-password"
              v-model="setupPassword"
              class="form-input"
              type="password"
              autocomplete="current-password"
              required
            />
          </FormField>
          <button class="btn btn-primary" type="submit" :disabled="busy || !setupPassword">
            Set it up
          </button>
        </form>
      </template>

      <template v-else>
        <p class="text-sm text-muted mb-3">
          Scan this with your authenticator app, or type the secret in by hand.
        </p>
        <div class="qr"><img :src="pending.qr" alt="Setup QR code" /></div>
        <div class="secret">
          <code>{{ pending.secret }}</code>
          <CopyButton :value="pending.secret" />
        </div>
        <form class="row" @submit.prevent="enable">
          <FormField label="Code from your app" for="totp-on">
            <input
              id="totp-on"
              v-model="code"
              class="form-input"
              inputmode="numeric"
              maxlength="6"
              autocomplete="one-time-code"
              placeholder="123456"
              required
            />
          </FormField>
          <button class="btn btn-primary" type="submit" :disabled="busy || !code">Turn on</button>
        </form>
        <button class="btn btn-secondary btn-sm" :disabled="busy" @click="pending = null">
          Cancel
        </button>
      </template>
    </div>
    <RecoveryCodes v-if="shownCodes" :codes="shownCodes" @close="shownCodes = null" />
  </div>
</template>

<style scoped>
.qr {
  display: flex;
  justify-content: center;
  margin-bottom: 14px;
}

/* White behind it whatever the theme: a camera reads it. */
.qr img {
  width: 220px;
  height: 220px;
  border: 1px solid var(--border-primary);
  border-radius: var(--radius-sm);
  background: #fff;
}

.secret {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 16px;
}

.secret code {
  flex: 1;
  padding: 8px 10px;
  border-radius: var(--radius-sm);
  background: var(--bg-tertiary);
  font-size: 12px;
  word-break: break-all;
}

.recovery {
  padding-bottom: 16px;
  margin-bottom: 16px;
  border-bottom: 1px solid var(--border-primary);
}

.row {
  display: flex;
  align-items: flex-end;
  gap: 12px;
}

.row :deep(.form-group) {
  flex: 1;
  margin-bottom: 0;
}
</style>
