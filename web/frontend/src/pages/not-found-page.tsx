import { Link } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { Home } from 'lucide-react'
import { useI18n } from '@/contexts/i18n-context'

export function NotFoundPage() {
  const { t } = useI18n()
  return (
    <div className="min-h-screen flex items-center justify-center corporate-background p-4">
      <div className="text-center space-y-6 animate-fade-up">
        <div className="font-display text-7xl font-bold tracking-tight text-brand-accent/30">404</div>
        <p className="text-lg text-muted-foreground">{t('not_found.title')}</p>
        <Link to="/"><Button size="lg"><Home className="w-4 h-4 mr-2" /> {t('not_found.go_home')}</Button></Link>
      </div>
    </div>
  )
}
