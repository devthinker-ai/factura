import { cn } from '@/lib/utils'
import { ButtonHTMLAttributes, forwardRef } from 'react'

export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: 'default' | 'secondary' | 'outline' | 'ghost' | 'destructive'
  size?: 'default' | 'sm' | 'lg' | 'icon'
}

const variants: Record<NonNullable<ButtonProps['variant']>, string> = {
  default:
    'bg-primary text-primary-foreground border border-border shadow-sm hover:bg-primary/90 active:scale-[0.98]',
  secondary:
    'bg-secondary text-secondary-foreground hover:bg-secondary/80 active:scale-[0.98]',
  outline:
    'border border-border bg-transparent hover:bg-secondary active:scale-[0.98]',
  ghost: 'hover:bg-secondary hover:text-foreground',
  destructive:
    'bg-destructive text-destructive-foreground border border-border shadow-sm hover:bg-destructive/90 active:scale-[0.98]',
}

const sizes: Record<NonNullable<ButtonProps['size']>, string> = {
  default: 'h-8 px-3 py-1.5',
  sm: 'h-7 rounded-md px-2.5 text-xs',
  lg: 'h-9 rounded-md px-5',
  icon: 'h-7 w-7',
}

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant = 'default', size = 'default', ...props }, ref) => (
    <button
      ref={ref}
      className={cn(
        'inline-flex items-center justify-center gap-1.5 whitespace-nowrap rounded-md text-sm font-medium transition-[color,background-color,transform,box-shadow] duration-150 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:pointer-events-none disabled:opacity-50',
        variants[variant],
        sizes[size],
        className,
      )}
      {...props}
    />
  ),
)
Button.displayName = 'Button'
