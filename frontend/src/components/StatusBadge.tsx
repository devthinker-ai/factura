import { Badge } from '@/components/ui/badge'

export function StatusBadge({ status }: { status: string }) {
  const s = status.toLowerCase()
  if (s === 'valid' || s === 'delivered') {
    return <Badge variant="success">{status}</Badge>
  }
  if (s === 'invalid' || s === 'sending') {
    return <Badge variant="warning">{status}</Badge>
  }
  if (s === 'parse_error' || s === 'failed') {
    return <Badge variant={s === 'failed' ? 'destructive' : 'secondary'}>{status}</Badge>
  }
  if (s === 'accepted') {
    return <Badge variant="info">{status}</Badge>
  }
  return <Badge variant="outline">{status}</Badge>
}
