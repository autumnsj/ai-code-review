import { useState } from 'react'
import {
  App, Button, Form, Input, Modal, Popconfirm, Space, Switch, Table, Tag, Typography, Alert,
} from 'antd'
import { PlusOutlined, DeleteOutlined, UserAddOutlined } from '@ant-design/icons'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { memberApi, Member } from '../../api/members'

export default function MembersPage() {
  const qc = useQueryClient()
  const { message } = App.useApp()
  const { data, isLoading } = useQuery({ queryKey: ['members'], queryFn: memberApi.list })
  const { data: unknowns } = useQuery({ queryKey: ['members-unknown'], queryFn: memberApi.unknown })
  const [open, setOpen] = useState(false)
  const [editing, setEditing] = useState<Member | null>(null)
  const [prefillLogin, setPrefillLogin] = useState<string | undefined>()

  const remove = useMutation({
    mutationFn: memberApi.remove,
    onSuccess: () => {
      message.success('已删除')
      qc.invalidateQueries({ queryKey: ['members'] })
    },
  })

  const openCreate = (login?: string) => {
    setEditing(null)
    setPrefillLogin(login)
    setOpen(true)
  }
  const openEdit = (m: Member) => {
    setEditing(m)
    setPrefillLogin(undefined)
    setOpen(true)
  }

  return (
    <div>
      <Space style={{ marginBottom: 16, justifyContent: 'space-between', width: '100%' }}>
        <Typography.Title level={3} style={{ margin: 0 }}>成员备注</Typography.Title>
        <Button type="primary" icon={<PlusOutlined />} onClick={() => openCreate()}>添加备注</Button>
      </Space>

      <Typography.Paragraph type="secondary">
        Git 提交中的作者是平台账号（login），通常不实名。这里把 login 映射到真实姓名 / 团队 / 备注，
        会在「作者排行」等统计中展示。
      </Typography.Paragraph>

      {unknowns && unknowns.length > 0 && (
        <Alert
          style={{ marginBottom: 16 }}
          type="info"
          showIcon
          message={`发现 ${unknowns.length} 个尚未备注的账号`}
          description={
            <Space wrap>
              {unknowns.map(u => (
                <Tag
                  key={u}
                  icon={<UserAddOutlined />}
                  color="blue"
                  style={{ cursor: 'pointer' }}
                  onClick={() => openCreate(u)}
                >
                  {u}
                </Tag>
              ))}
            </Space>
          }
        />
      )}

      <Table
        rowKey="id"
        loading={isLoading}
        dataSource={data ?? []}
        pagination={false}
        columns={[
          { title: '平台账号', dataIndex: 'git_login', width: 200, render: (t: string) => <Typography.Text code>@{t}</Typography.Text> },
          { title: '真实姓名', dataIndex: 'display_name', width: 160, render: (t: string) => t || '-' },
          { title: '团队', dataIndex: 'team', width: 140, render: (t: string) => t || '-' },
          { title: '备注', dataIndex: 'note', ellipsis: true },
          {
            title: '状态', dataIndex: 'active', width: 90,
            render: (v: boolean) => v ? <Tag color="green">启用</Tag> : <Tag>停用</Tag>,
          },
          {
            title: '操作', width: 150,
            render: (_: unknown, r: Member) => (
              <Space>
                <Button size="small" onClick={() => openEdit(r)}>编辑</Button>
                <Popconfirm title="删除该备注？" onConfirm={() => remove.mutate(r.id)}>
                  <Button size="small" danger icon={<DeleteOutlined />} />
                </Popconfirm>
              </Space>
            ),
          },
        ]}
      />

      <MemberModal
        open={open}
        editing={editing}
        prefillLogin={prefillLogin}
        onClose={() => setOpen(false)}
      />
    </div>
  )
}

function MemberModal({
  open, editing, prefillLogin, onClose,
}: {
  open: boolean
  editing: Member | null
  prefillLogin?: string
  onClose: () => void
}) {
  const qc = useQueryClient()
  const { message } = App.useApp()
  const [form] = Form.useForm()

  const create = useMutation({ mutationFn: memberApi.create })
  const update = useMutation({ mutationFn: (v: { id: number; body: any }) => memberApi.update(v.id, v.body) })

  const onOk = async () => {
    const v = await form.validateFields()
    if (editing) {
      update.mutate(
        { id: editing.id, body: { display_name: v.display_name, team: v.team, note: v.note, active: v.active } },
        {
          onSuccess: () => {
            message.success('已保存'); qc.invalidateQueries({ queryKey: ['members'] }); onClose()
          },
          onError: (e: any) => message.error(e?.response?.data?.error || '保存失败'),
        },
      )
    } else {
      create.mutate(
        {
          git_login: v.git_login, display_name: v.display_name, team: v.team, note: v.note,
          active: v.active ?? true,
        },
        {
          onSuccess: () => {
            message.success('已添加')
            qc.invalidateQueries({ queryKey: ['members'] })
            qc.invalidateQueries({ queryKey: ['members-unknown'] })
            onClose()
          },
          onError: (e: any) => message.error(e?.response?.data?.error || '添加失败'),
        },
      )
    }
  }

  return (
    <Modal
      title={editing ? `编辑备注 @${editing.git_login}` : '添加成员备注'}
      open={open}
      onCancel={onClose}
      onOk={onOk}
      confirmLoading={create.isPending || update.isPending}
      destroyOnClose
      forceRender
    >
      <Form
        form={form}
        layout="vertical"
        initialValues={
          editing
            ? { ...editing }
            : { git_login: prefillLogin ?? '', active: true }
        }
      >
        <Form.Item
          label="平台账号 (git login)"
          name="git_login"
          rules={[{ required: true, message: '请输入平台账号' }]}
        >
          <Input placeholder="如 octocat" disabled={!!editing} addonBefore="@" />
        </Form.Item>
        <Form.Item label="真实姓名" name="display_name">
          <Input placeholder="张三" />
        </Form.Item>
        <Form.Item label="团队" name="team">
          <Input placeholder="基础架构组" />
        </Form.Item>
        <Form.Item label="备注" name="note">
          <Input.TextArea rows={2} placeholder="可选" />
        </Form.Item>
        <Form.Item label="启用" name="active" valuePropName="checked">
          <Switch checkedChildren="启用" unCheckedChildren="停用" />
        </Form.Item>
      </Form>
    </Modal>
  )
}
