import { useEffect, useMemo, useState } from 'react'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { useI18n } from '@/contexts/i18n-context'
import type { SystemAddressOption } from '@/types/domain'

const manualValue = '__manual__'

interface Props {
  value: string
  options: SystemAddressOption[]
  onChange: (value: string) => void
  placeholder?: string
  family?: 'ipv4' | 'ipv6'
}

function optionLabel(option: SystemAddressOption, t: (key: string) => string): string {
  const parts = [option.ip]
  if (option.interface) {
    parts.push(option.interface)
  }
  let label = parts.join(' · ')
  if (option.link_local) {
    label += ` (${t('admin.bgp_link_local')})`
  }
  return label
}

export function SystemAddressField({ value, options, onChange, placeholder, family = 'ipv4' }: Props) {
  const { t } = useI18n()
  const optionIPs = useMemo(() => options.map(option => option.ip), [options])
  const [manual, setManual] = useState(() => value !== '' && !optionIPs.includes(value))

  useEffect(() => {
    if (value !== '' && !optionIPs.includes(value)) {
      setManual(true)
    }
  }, [value, optionIPs])

  if (manual || options.length === 0) {
    return (
      <div className="space-y-2">
        <Input
          value={value}
          onChange={e => onChange(e.target.value)}
          placeholder={placeholder}
          className="font-data"
        />
        {options.length > 0 ? (
          <button
            type="button"
            className="text-xs text-brand hover:underline"
            onClick={() => {
              setManual(false)
              if (!optionIPs.includes(value)) {
                onChange('')
              }
            }}
          >
            {t('admin.bgp_pick_system_address')}
          </button>
        ) : (
          <p className="text-xs text-muted-foreground">
            {family === 'ipv6' ? t('admin.bgp_no_system_ipv6') : t('admin.bgp_no_system_ipv4')}
          </p>
        )}
      </div>
    )
  }

  return (
    <Select
      value={value || undefined}
      onValueChange={next => {
        if (next === manualValue) {
          setManual(true)
          return
        }
        onChange(next)
      }}
    >
      <SelectTrigger className="font-data">
        <SelectValue placeholder={placeholder ?? t('admin.bgp_select_peering_ip')} />
      </SelectTrigger>
      <SelectContent>
        {options.map(option => (
          <SelectItem key={`${option.interface ?? 'if'}-${option.ip}`} value={option.ip} className="font-data">
            {optionLabel(option, t)}
          </SelectItem>
        ))}
        <SelectItem value={manualValue}>{t('admin.bgp_enter_manually')}</SelectItem>
      </SelectContent>
    </Select>
  )
}
