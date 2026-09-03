// 揭示动效包装：渲染带 .reveal 类的元素，进入视口由 Layout 的 useReveal 统一加 .in
// 支持 as 指定标签/组件（div / blockquote / Link 等）
export default function Reveal({ as: Tag = 'div', className = '', children, ...rest }) {
  return (
    <Tag className={`reveal ${className}`.trim()} {...rest}>
      {children}
    </Tag>
  )
}
