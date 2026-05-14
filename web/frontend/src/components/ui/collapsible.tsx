import * as React from 'react'
import * as CollapsiblePrimitive from '@radix-ui/react-collapsible'

const Collapsible = CollapsiblePrimitive.Root
const CollapsibleTrigger = CollapsiblePrimitive.CollapsibleTrigger
const CollapsibleContent = React.forwardRef<HTMLDivElement, React.ComponentPropsWithoutRef<typeof CollapsiblePrimitive.CollapsibleContent>>((props, ref) => (
  <CollapsiblePrimitive.CollapsibleContent ref={ref} {...props} />
))

export { Collapsible, CollapsibleTrigger, CollapsibleContent }
