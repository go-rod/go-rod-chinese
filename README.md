# 概览

[![Go Reference](https://pkg.go.dev/badge/github.com/go-rod/go-rod-chinese.svg)](https://pkg.go.dev/github.com/go-rod/go-rod-chinese)
[![Discord Chat](https://img.shields.io/discord/719933559456006165.svg)][discord room]

## [教程文档](https://go-rod.github.io/) | [英文 API 参考文档](https://pkg.go.dev/github.com/go-rod/rod?tab=doc) | [中文 API 参考文档](https://pkg.go.dev/github.com/go-rod/go-rod-chinese?tab=doc) | [项目管理](https://github.com/orgs/go-rod/projects/1) | [FAQ](https://go-rod.github.io/#/faq/README)

Rod 是一个直接基于 [DevTools Protocol](https://chromedevtools.github.io/devtools-protocol) 高级驱动程序。
它是为网页自动化和爬虫而设计的，既可用于高级应用开发也可用于低级应用开发，高级开发人员可以使用低级包和函数来轻松地定制或建立他们自己的Rod版本，高级函数只是建立Rod默认版本的例子。

## 特性

- 链式上下文设计，直观地超时或取消长时间运行的任务
- 自动等待元素准备就绪
- 调试友好，自动输入跟踪，远程监控无头浏览器
- 所有操作都是线程安全的
- 自动查找或下载 [浏览器](lib/launcher)
- 高级的辅助程序像 WaitStable, WaitRequestIdle, HijackRequests, WaitDownload,等
- 两步式的 WaitEvent 设计，永远不会错过任何一个事件 ([工作原理](https://github.com/ysmood/goob))
- 正确地处理嵌套的iframe或影子DOM
- 崩溃后没有僵尸浏览器进程 ([工作原理](https://github.com/ysmood/leakless))
- [CI](https://github.com/go-rod/rod/actions) 100% 的测试覆盖率

## 关于中文 API 参考文档的说明

- 中文 API 参考文档中含有 `TODO` 的地方，表示目前的没有较好的翻译，如果有觉得很适合的翻译，请在中文仓库下提交 [issues](https://github.com/go-rod/go-rod-chinese/issues)/[discussions](https://github.com/go-rod/go-rod-chinese/discussions) 
- 翻译风格，翻译建议，翻译勘误，请在中文仓库下提交 [issues](https://github.com/go-rod/go-rod-chinese/issues)/[discussions](https://github.com/go-rod/go-rod-chinese/discussions) 
- 不建议将中文仓库的代码，使用在您的项目中，强烈建议使用[英文仓库](https://github.com/go-rod/rod)的代码。中文仓库仅供作为 API 文档中文版的参考
- 关于API文档的翻译情况：对于底层库封装出来的接口已经全部翻译，底层库目前仅翻译了一些和功能业务相关的，例如：Network，Page等
- 欢迎加入 rod 中文 API 参考文档的建设当中来

## 示例

首先请查看 [examples_test.go](examples_test.go), 然后查看 [examples](lib/examples) 文件夹.有关更详细的示例，请搜索单元测试。
例如 `HandleAuth`的使用， 你可以搜索所有 `*_test.go` 文件包含`HandleAuth`的，例如，使用 Github 在线搜索 [在仓库中搜索](https://github.com/go-rod/rod/search?q=HandleAuth&unscoped_q=HandleAuth)。
你也可以搜索 GitHub 的 [issues](https://github.com/go-rod/rod/issues) 或者 [discussions](https://github.com/go-rod/rod/discussions),这里记录了更多的使用示例。

[这里](lib/examples/compare-chromedp) 是一个 rod 和 chromedp 的比较。

如果你有疑问，可以提 [issues](https://github.com/go-rod/rod/issues)/[discussions](https://github.com/go-rod/rod/discussions) 或者加入 [chat room][discord room]。

## 加入我们

我们非常欢迎你的帮助! 即使只是打开一个问题，提出一个问题，也可能大大帮助别人。

在你提出问题之前，请阅读 [如何聪明的提问](http://www.catb.org/~esr/faqs/smart-questions.html)。

我们使用 Github 项目来管理任务，你可以在[这里](https://github.com/orgs/go-rod/projects/1)看到这些问题的优先级和进展。

如果你想为项目作出贡献，请阅读 [Contributor Guide](.github/CONTRIBUTING.md)。

[discord room]: https://discord.gg/CpevuvY
