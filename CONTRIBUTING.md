# 贡献指南

感谢你考虑为 Gconf 做出贡献！

## 如何贡献

### 报告问题

如果你发现了 bug 或有功能建议：

1. 检查 [Issues](https://github.com/nicexiaonie/gconf/issues) 是否已有相关问题
2. 如果没有，创建新 Issue，请包含：
   - 清晰的标题和描述
   - 重现步骤（对于 bug）
   - 期望行为和实际行为
   - Go 版本和操作系统信息
   - 相关代码片段或错误信息

### 提交代码

1. **Fork 仓库**
   ```bash
   # Fork 到你的账号，然后 clone
   git clone https://github.com/YOUR_USERNAME/gconf.git
   cd gconf
   ```

2. **创建分支**
   ```bash
   git checkout -b feature/your-feature-name
   # 或
   git checkout -b fix/your-bug-fix
   ```

3. **编写代码**
   - 遵循现有代码风格
   - 添加必要的注释
   - 确保代码格式化: `make fmt`

4. **添加测试**
   ```bash
   # 为新功能添加测试
   # 确保所有测试通过
   make test
   ```

5. **提交更改**
   ```bash
   git add .
   git commit -m "feat: 添加新功能"
   # 或
   git commit -m "fix: 修复某个问题"
   ```

   提交信息格式：
   - `feat:` 新功能
   - `fix:` 修复 bug
   - `docs:` 文档更新
   - `test:` 测试相关
   - `refactor:` 重构
   - `style:` 代码格式调整
   - `chore:` 构建/工具相关

6. **推送并创建 Pull Request**
   ```bash
   git push origin feature/your-feature-name
   ```
   
   然后在 GitHub 上创建 Pull Request。

## 开发指南

### 环境要求

- Go 1.14 或更高版本
- Git

### 开发流程

1. **安装依赖**
   ```bash
   make deps
   ```

2. **运行测试**
   ```bash
   # 运行所有测试
   make test
   
   # 详细输出
   make test-verbose
   
   # 生成覆盖率报告
   make test-coverage
   ```

3. **运行示例**
   ```bash
   # 运行所有示例
   make example
   
   # 运行特定示例
   make run-basic
   make run-advanced
   make run-env
   ```

4. **代码检查**
   ```bash
   # 格式化代码
   make fmt
   
   # 运行 linter
   make lint
   ```

5. **性能测试**
   ```bash
   make bench
   ```

### 代码规范

- 使用 `gofmt` 格式化代码
- 遵循 [Effective Go](https://golang.org/doc/effective_go.html)
- 公开的函数和类型必须有文档注释
- 保持函数简洁，单一职责
- 错误处理要明确

### 测试要求

- 新功能必须包含测试
- 测试覆盖率应保持在合理水平
- 测试应该：
  - 清晰表达意图
  - 独立运行
  - 可重复执行

### 文档

- 更新相关文档（README、QUICKSTART 等）
- 为新功能添加示例
- 保持中英文文档同步

## Pull Request 检查清单

在提交 PR 前，请确保：

- [ ] 代码已格式化 (`make fmt`)
- [ ] 所有测试通过 (`make test`)
- [ ] 添加了必要的测试
- [ ] 更新了相关文档
- [ ] 提交信息清晰明确
- [ ] PR 描述清楚说明了变更内容

## 代码审查

所有 PR 都需要经过代码审查。审查者会关注：

- 代码质量和可读性
- 测试覆盖度
- 文档完整性
- 是否符合项目方向

## 行为准则

- 尊重他人
- 欢迎新手
- 保持友好和专业
- 接受建设性批评

## 许可证

提交代码即表示你同意将代码以 MIT 许可证授权。

## 获取帮助

如有疑问，可以：

- 提交 Issue
- 在 PR 中提问
- 查看现有代码和文档

## 致谢

感谢所有贡献者！你们的参与让 Gconf 变得更好。

---

**Happy Contributing! 🎉**

