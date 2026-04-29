import { Message } from './types';
import { randomBorderColor, renderMarkdown, highlightCode, renderMermaid, renderECharts, renderMath, clearContainer, previewImage } from './utils';

export function createMessageElement(message: Message): HTMLElement {
  const messageElement = document.createElement('div');
  messageElement.classList.add('message');

  if (message.role === 'user') {
    messageElement.classList.add('user-message');
  } else {
    messageElement.classList.add('assistant-message');
  }

  messageElement.style.borderColor = randomBorderColor();

  const header = document.createElement('div');
  header.className = 'message-header';
  header.textContent = message.role === 'user' ? '你' : '月华';

  const contentDiv = document.createElement('div');
  contentDiv.className = 'message-content';

  messageElement.appendChild(header);
  messageElement.appendChild(contentDiv);

  return messageElement;
}

export async function renderMessage(message: Message, container: HTMLElement): Promise<HTMLElement> {
  const messageElement = createMessageElement(message);
  const contentDiv = messageElement.querySelector('.message-content') as HTMLElement;

  if (message.imageUrls && message.imageUrls.length > 0) {
    const imagesContainer = document.createElement('div');
    imagesContainer.className = 'images-container';

    for (const imageUrl of message.imageUrls) {
      const imgContainer = document.createElement('div');
      imgContainer.className = 'labeled-image-container';

      const img = document.createElement('img');
      img.src = imageUrl;
      img.className = 'image-just-drawn';
      img.alt = typeof message.content === 'string' ? message.content : '图片';
      img.onerror = () => {
        img.src = `/read/resources/placeholder/blank-0${Math.floor(Math.random() * 3)}.png`;
      };
      img.onclick = () => previewImage(imageUrl, typeof message.content === 'string' ? message.content : '图片');

      imgContainer.appendChild(img);
      imagesContainer.appendChild(imgContainer);
    }

    contentDiv.appendChild(imagesContainer);
  } else if (message.imageUrl) {
    const imgContainer = document.createElement('div');
    imgContainer.className = 'labeled-image-container';

    const img = document.createElement('img');
    img.src = message.imageUrl;
    img.className = 'image-just-drawn';
    img.alt = typeof message.content === 'string' ? message.content : '图片';
    img.onerror = () => {
      img.src = `/read/resources/placeholder/blank-0${Math.floor(Math.random() * 3)}.png`;
    };
    img.onclick = () => previewImage(message.imageUrl!, typeof message.content === 'string' ? message.content : '图片');

    imgContainer.appendChild(img);
    contentDiv.appendChild(imgContainer);
  }

  if (typeof message.content === 'string' && message.content) {
    const markdownContent = document.createElement('div');
    markdownContent.className = 'markdown-content';
    markdownContent.innerHTML = await renderMarkdown(message.content);
    contentDiv.appendChild(markdownContent);

    highlightCode(markdownContent);
    await renderMermaid(markdownContent);
    renderECharts(markdownContent);
    await renderMath(markdownContent);
  }

  container.appendChild(messageElement);
  container.scrollTo({ top: container.scrollHeight, behavior: 'smooth' });

  return messageElement;
}

export async function renderAllMessages(container: HTMLElement, messages: Message[], clearFirst = true): Promise<void> {
  if (clearFirst) {
    clearContainer(container);
  }

  for (const message of messages) {
    await renderMessage(message, container);
  }
}
