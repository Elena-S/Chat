window.onload = function(event) {
    chooseAuthOption = (event) => {
        let currentItem;
        for (let item = event.target; item != event.currentTarget; item = item.parentElement) {
            if (item.classList.contains('auth-option')) {
                currentItem = item;
                break;
            }
        } 

        if (!currentItem || currentItem.classList.contains('auth-option-active')) {
            return;
        }
        
        for (let item = event.currentTarget.firstElementChild; item; item = item.nextElementSibling) {
            item.classList.toggle('auth-option-active');
            item.classList.toggle('auth-option-inactive');
        }
        
        let handler;
        document.authentication.action = window.location.toString();
        if (currentItem.getAttribute('id') == 'register') {
            document.getElementById("registration").value = "on";
            handler = (item) => {
                item.classList.remove('invisible');
                const itemInput = item.querySelector('input');
                if (itemInput) {
                    itemInput.setAttribute('required', 'true');
                }
            };

        } else {
            document.getElementById("registration").value = false;
            handler = (item) => {
                item.classList.add('invisible');
                const itemInput = item.querySelector('input[required]');
                if (itemInput) {
                    itemInput.removeAttribute('required');
                }
            };
        }

        const items = document.querySelectorAll('[data-optional]');
        items.forEach(handler);
    }

    document.getElementById('auth-options').addEventListener('click', chooseAuthOption);
}