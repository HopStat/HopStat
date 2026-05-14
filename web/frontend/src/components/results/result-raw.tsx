import { ChevronDown } from 'lucide-react'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import { useI18n } from '@/contexts/i18n-context'

interface Props { raw: string }

export function ResultRaw({ raw }: Props) {
  const { t } = useI18n()
  return (
    <Collapsible>
      <CollapsibleTrigger className="flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground transition-colors">
        <ChevronDown className="w-4 h-4" /> {t('result.raw')}
      </CollapsibleTrigger>
      <CollapsibleContent>
        <pre className="mt-2 p-4 rounded-md bg-muted text-xs overflow-auto max-h-96 font-mono whitespace-pre-wrap">
          {raw}
        </pre>
      </CollapsibleContent>
    </Collapsible>
  )
}
