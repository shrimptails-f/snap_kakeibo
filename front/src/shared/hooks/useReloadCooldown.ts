import { useEffect, useState } from 'react'

export const RELOAD_COOLDOWN_MS = 3000

export function useReloadCooldown() {
  const [cooldownUntil, setCooldownUntil] = useState<number | null>(null)
  const [cooldownRemainingMs, setCooldownRemainingMs] = useState(0)

  useEffect(() => {
    if (cooldownUntil === null) return
    const deadline = cooldownUntil
    function updateRemaining() {
      const remaining = Math.max(0, deadline - Date.now())
      setCooldownRemainingMs(remaining)
      if (remaining === 0) setCooldownUntil(null)
    }
    updateRemaining()
    const timer = setInterval(updateRemaining, 100)
    return () => clearInterval(timer)
  }, [cooldownUntil])

  function startCooldown(): boolean {
    if (cooldownUntil !== null) return false
    setCooldownRemainingMs(RELOAD_COOLDOWN_MS)
    setCooldownUntil(Date.now() + RELOAD_COOLDOWN_MS)
    return true
  }

  return { startCooldown, isCoolingDown: cooldownUntil !== null, cooldownRemainingMs }
}
