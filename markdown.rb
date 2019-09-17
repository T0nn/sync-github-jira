#!/usr/bin/env ruby

require 'commonmarker'

# function paramter ??
doc = CommonMarker.render_doc(ARGF.read, :DEFAULT, [:strikethrough, :table])

# PP.pp(doc)
class MyJiraRenderer < CommonMarker::Renderer

  # escape() jira html href
  # html inline_html
  # footnote

  def initialize
    super
    @listItemLevel = 0
    @listItemOrder = false
  end

  def render(node)
    super(node)
  end

  def document(_node)
    out(:children)
  end
  
  def text(node)
    out(node.string_content)
  end

  def blockquote(node)
    # old_in_tight = @in_tight
    # @in_tight = true
    block do
      container("{quote}\n", '{quote}') do
        out(:children)
      end
    end
    # @in_tight = old_in_tight
  end

  def paragraph(node)
    if @in_tight # paragraphs in a loose list are wrapped in <p> tags
      out(:children)
    else
      block do
        out(:children)
      end
      blocksep # paragraph seprated by newline
    end
  end

  def header(node)
    block do
      out("h"+node.header_level.to_s+ ". ", :children)
    end
  end

  def hrule(node)
    block do
      out("----")
    end
  end

  # linebreak softbreak ??
  def linebreak(node)
    out("\\\n")
  end

  def softbreak(_)
    out("\n")
  end

  def emph(_)
    out('_', :children, '_')
  end

  def strong(_)
    out('*', :children, '*')
  end

  def strikethrough(_)
    out('-', :children, '-')
  end

  def link(node)
    out('[', :children, !node.first_child.nil? ? '|':'',node.url, ']')
  end

  def image(node)
    out('!', node.url, '!')
  end

  def code(node)
    out("{{", node.string_content, "}}")
  end

  def code_block(node)
    validLanguage = Array["actionscript", "ada", "applescript", "bash", "c", "c#", "c++", "cpp", "css", "erlang", "go", "groovy", "haskell", "html", "java", "javascript", "js", "json", "lua", "none", "nyan", "objc", "perl", "php", "python", "r", "rainbow", "ruby", "scala", "sh", "sql", "swift", "visualbasic", "xml", "yaml"]
    block do

      if node.fence_info && !node.fence_info.empty?
        language = node.fence_info.split(/\s+/)[0]
        out('{code:', validLanguage.include?(language)? language:'none', '}')
      else
        out("{code:none}")
      end
      block do
        out(node.string_content)
      end
      out("{code}")
    end
  end

  # itemLevel

  def list(node)
    old_in_tight = @in_tight
    # @in_tight = node.list_tight
    @in_tight = true

    old_listItemLevel = @listItemLevel
    old_listItemOrder = @listItemOrder
    @listItemLevel += 1
    @listItemOrder = (node.list_type == :ordered_list)
    @listItemOrder = false # hack due to jira crack order list render

    block do
      out(:children)
    end
    
    @listItemLevel = old_listItemLevel
    @listItemOrder = old_listItemOrder
    blocksep if @listItemLevel == 0 

    @in_tight = old_in_tight
  end

  def list_item(node)
    block do
      itemTag = (@listItemOrder ? '#':'*') * @listItemLevel
      container(itemTag+' ', '') do
        out(:children)
      end
    end
  end

  def table(node)
    block do
      out(:children) 
    end
    blocksep
  end

  def table_header(node)
    @in_header = true
    block do
      out(:children)
    end
    @in_header = false
  end

  def table_row(node)
    block do
      out(:children)
    end
  end

  def table_cell(node)
    out(@in_header ? '||':'|', :children)
    if node.next.nil?
      out(@in_header ? '||':'|')
    end
  end

  def html(node)
    block do
      if option_enabled?(:SAFE)
        out('<!-- raw HTML omitted -->')
      else
        # filter out raw HTML comment
        out(commentfilter(tagfilter(node.string_content)))
      end
    end
  end

  def inline_html(node)
    if option_enabled?(:SAFE)
      out('<!-- raw HTML omitted -->')
    else
      # filter out raw HTML comment
      out(commentfilter(tagfilter(node.string_content)))
    end
  end

  def footnote_reference(node)
    return # not handle currently

    out("<sup class=\"footnote-ref\"><a href=\"#fn#{node.string_content}\" id=\"fnref#{node.string_content}\">#{node.string_content}</a></sup>")
  end

  def footnote_definition(_)
    return # not handle currently

    if !@footnote_ix
      out("<section class=\"footnotes\">\n<ol>\n")
      @footnote_ix = 0
    end

    @footnote_ix += 1
    out("<li id=\"fn#@footnote_ix\">\n", :children)
    if out_footnote_backref
      out("\n")
    end
    out("</li>\n")
    # in the end of output, out("</ol>\n</section>\n") 
  end

  private

    def commentfilter(str)
      str.gsub(/<!--(.*?)-->/, '')
    end

    def out_footnote_backref
      return false if @written_footnote_ix == @footnote_ix
      @written_footnote_ix = @footnote_ix

      out("<a href=\"#fnref#@footnote_ix\" class=\"footnote-backref\">↩</a>")
      true
    end

end

myrenderer = MyJiraRenderer.new
STDOUT.write(myrenderer.render(doc))
