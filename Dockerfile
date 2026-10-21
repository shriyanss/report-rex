# Report Rex — Runtime Image
# Base: pandoc/latex already bundles pandoc + TeX Live + XeLaTeX
# The -ubuntu variant publishes both linux/amd64 and linux/arm64 manifests
# (unlike the default Alpine-based tag, which is amd64-only), so this image
# builds and runs natively on arm64 Linux hosts — no QEMU emulation needed.
FROM pandoc/latex:3.6-ubuntu

# Install Python 3 and pip
# poppler-utils provides pdftoppm, used to rasterize chart TikZ into PNGs for
# the DOCX pipeline (DOCX has no LaTeX renderer, unlike the PDF pipeline's
# raw TikZ blocks).
# qpdf provides `--password` PDF encryption (AES-256), run post-render.
# tesseract-ocr powers `rex ocr` (screenshot → code block).
RUN apt-get update && apt-get install -y --no-install-recommends \
      python3 python3-pip python3-dev fontconfig curl poppler-utils qpdf \
      tesseract-ocr \
    && rm -rf /var/lib/apt/lists/*

# Install vale (pinned version) for prose/grammar linting
ARG VALE_VERSION=3.17.1
RUN case "$(uname -m)" in \
      x86_64)  VALE_ARCH=64-bit ;; \
      aarch64) VALE_ARCH=arm64 ;; \
      *) echo "unsupported architecture: $(uname -m)" >&2; exit 1 ;; \
    esac \
    && curl -fsSL "https://github.com/vale-cli/vale/releases/download/v${VALE_VERSION}/vale_${VALE_VERSION}_Linux_${VALE_ARCH}.tar.gz" \
      | tar -xz -C /usr/local/bin vale

# Install additional LaTeX packages required by the default template.
# pandoc/latex:3.6-ubuntu ships TeX Live 2025 (the Alpine 3.6 tag shipped
# 2024 — the two base variants pin different upstream TL releases); tlmgr
# refuses to mix a 2025 local install with a repository that doesn't cover
# that version, so we pin to the frozen TL2025 archive instead of TL2024.
# Now that TL2025 is itself frozen (TL2026 has since released), the
# bundled tlmgr also refuses to talk to that archive until it self-updates
# against it first; --verify-repo=none skips GPG verification because the
# frozen archive's signing key has since expired.
RUN TLREPO="https://texlive.info/historic/systems/texlive/2025/tlnet-final/" && \
    tlmgr --repository "$TLREPO" update --self --verify-repo=none && \
    tlmgr install \
        tcolorbox fontawesome5 titlesec fancyhdr geometry xcolor pgf \
        environ trimspaces draftwatermark everypage listings pgfplots \
        booktabs microtype unicode-math enumitem parskip tex-gyre \
        pdfcol tikzfill \
        tabularray ninecolors pgf-pie framed fvextra caption float \
        lineno upquote \
        standalone varwidth preview \
        --repository "$TLREPO" --verify-repo=none

# tcolorbox 6.9.0 (whatever this frozen archive happens to pin) unconditionally
# calls PDF-tagging kernel macros (\NewStructureName and friends) that only
# exist once a document opts into \DocumentMetadata{testphase=tagpdf} — and
# that machinery is itself broken in this exact snapshot (tagpdf 0.99y calls
# \IfPDFManagementActiveF, which pdfmanagement-testphase never defines; even
# patching that in by hand reveals a further missing \socket_if_exist:nF, and
# beyond that a broken tcolorbox-internal tagging counter — confirmed by
# manually tracing the load sequence on real hardware, three layers deep).
# None of report-rex's templates use real PDF tagging, so rather than chase
# that rabbit hole, pin tcolorbox to the last release before this tagging
# code was added (6.9.0, 2025/11/28) — 6.8.0 (2025/09/09) predates it and
# compiles cleanly, same as the older tcolorbox TL2024's Alpine tag shipped.
RUN TCOLORBOX_DIR="$(dirname "$(kpsewhich tcolorbox.sty)")" && \
    curl -fsSL "https://github.com/T-F-S/tcolorbox/archive/refs/tags/v6.8.0.tar.gz" \
      | tar -xz -C "$TCOLORBOX_DIR" --strip-components=4 --overwrite \
        tcolorbox-6.8.0/tex/latex/tcolorbox

# Register tex-gyre OTF fonts so XeLaTeX/fontspec resolves the default fonts
RUN mkdir -p /usr/share/fonts/tex-gyre && \
    find /opt/texlive/texdir/texmf-dist/fonts/opentype/public/tex-gyre/ -name "*.otf" \
      -exec cp {} /usr/share/fonts/tex-gyre/ \; && \
    fc-cache -fv

# Install the core Python package
COPY core/ /opt/report-rex-core/
RUN pip3 install --no-cache-dir --break-system-packages /opt/report-rex-core/

# Copy templates (including any fonts under assets/fonts/)
COPY templates/ /opt/report-rex/templates/

# Copy vale config + vendored styles (prose/grammar linting)
COPY .vale.ini /opt/report-rex/.vale.ini
COPY styles/ /opt/report-rex/styles/

# Register any fonts shipped with templates
RUN find /opt/report-rex/templates -name '*.ttf' -o -name '*.otf' | \
    xargs -I{} cp {} /usr/local/share/fonts/ 2>/dev/null; \
    fc-cache -f 2>/dev/null; \
    true

WORKDIR /report

# Default templates root points to the bundled templates
ENV REPORT_REX_TEMPLATES=/opt/report-rex/templates

ENTRYPOINT ["report-rex"]
CMD ["--help"]
